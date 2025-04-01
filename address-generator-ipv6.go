package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/big"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
)

type Arguments struct {
	PrefixFile       string
	TargetPrefixSize int
	TestMode         bool
	Seed             int64
	Limit            int
}

// Linear Congruential Generator
// as described in https://stackoverflow.com/a/53551417
type lcg_state struct {
	value      int64
	offset     int64
	multiplier int64
	modulus    int64
	max        int64
	max_iter   int64
	found      int64
}

type Lcg struct {
	lcg_state
}

func (lcg *Lcg) Init(seed int64, stop int64) {
	// Seed range with a random integer.
	//fmt.Println("%d",seed)
	lcg.value = rand.Int63n(seed)
	lcg.offset = rand.Int63n(seed)*2 + 1                                  // Pick a random odd-valued offset.
	lcg.multiplier = 4*(seed/4) + 1                                       // Pick a multiplier 1 greater than a multiple of 4
	lcg.modulus = int64(math.Pow(2, math.Ceil(math.Log2(float64(seed))))) // Pick a modulus just big enough to generate all numbers (power of 2)
	lcg.found = 0                                                         // Track how many random numbers have been returned
	lcg.max = seed
	lcg.max_iter = stop
}

func (lcg *Lcg) Next() int64 {
	for lcg.value >= lcg.max {
		lcg.value = (lcg.value*lcg.multiplier + lcg.offset) % lcg.modulus
	}
	lcg.found += 1
	value := lcg.value
	// Calculate the next value in the sequence.
	lcg.value = (lcg.value*lcg.multiplier + lcg.offset) % lcg.modulus
	return value
}

func (lcg *Lcg) Has_next() bool {
	return lcg.found < lcg.max
}

func (lcg *Lcg) Max_iterations_reached() bool {
	return lcg.found >= lcg.max_iter
}

// Struct to hold subnet generation state for each prefix
type subnetGenerator struct {
	baseIP        *big.Int
	subnetMask    net.IPMask
	subnetCount   int
	currentIndex  int
	increment     *big.Int
	numRandBlocks int
	n             int
	lcg           Lcg
	done          bool
	mode          rune
}

func GenerateRandom16Bit() uint16 {
	return uint16(rand.Intn(1 << 16))
}
func IncrementLastNBlocks(num *big.Int, numBlocks int) *big.Int {
	// Mask and shift values to isolate each 16-bit block
	var mask uint64 = 0xFFFF
	newNum := new(big.Int).Set(num) // Make a copy to modify
	oldNum := new(big.Int).Set(num) // Make a copy to modify

	for i := 0; i < numBlocks; i++ {
		// Isolate the 16-bit block
		shift := uint(i * 16)
		block := uint64(oldNum.Rsh(oldNum, shift).Uint64() & mask)

		// Generate a random increment and add to the block (mod 2^16)
		increment := uint64(GenerateRandom16Bit())
		newBlock := (block + increment) & mask

		newNum.Or(newNum, new(big.Int).SetUint64(newBlock<<shift))
	}

	return newNum
}

// Generate the next subnet address for this prefix
func (gen *subnetGenerator) nextSubnet() string {
	if gen.currentIndex >= gen.subnetCount || (gen.n > 0 && gen.currentIndex >= gen.n) {
		gen.done = true // Mark generator as done
		return ""       // No more subnets to generate
	}
	var nextValue int64

	if gen.lcg.Has_next() {
		nextValue = gen.lcg.Next()
	} else {
		gen.done = true
		return ""
	}

	subnetIP := new(big.Int).Add(gen.baseIP, new(big.Int).Mul(gen.increment, big.NewInt(nextValue)))
	subnetIPRand := IncrementLastNBlocks(subnetIP, gen.numRandBlocks)
	gen.currentIndex++
	switch gen.mode {
	case 'n':
		return bigIntToIP(subnetIP).Mask(gen.subnetMask).String()
	case 'r':
		return bigIntToIP(subnetIPRand).String()
	default:
		return bigIntToIP(subnetIP).Mask(gen.subnetMask).String() + "\n" + bigIntToIP(subnetIPRand).String()
	}
}

// Create a subnet generator for a given prefix
func createSubnetGenerator(prefix string, targetPrefixSize int, n int, mode rune) (*subnetGenerator, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, fmt.Errorf("Invalid prefix: %s", prefix)
	}

	networkPrefixSize, _ := network.Mask.Size()

	var lcg Lcg

	if networkPrefixSize > targetPrefixSize {
		// If the input prefix size is larger than the target, calculate the supernet (larger than the original)
		supernetMask := net.CIDRMask(targetPrefixSize, 128)
		supernet := &net.IPNet{
			IP:   network.IP.Mask(supernetMask),
			Mask: supernetMask,
		}
		return &subnetGenerator{
			baseIP:        ipToBigInt(supernet.IP),
			subnetMask:    supernet.Mask,
			subnetCount:   1, // Just one supernet
			currentIndex:  0,
			increment:     big.NewInt(0), // No increment needed
			numRandBlocks: (128 - targetPrefixSize) / 16,
			n:             n,
			lcg:           lcg,
			done:          false,
			mode:          mode,
		}, nil
	}

	// Subnet calculation for targetPrefixSize
	subnetMask := net.CIDRMask(targetPrefixSize, 128)
	baseIPInt := ipToBigInt(network.IP)

	// Number of subnets we need to generate
	subnetCount := 1 << (targetPrefixSize - networkPrefixSize)
	// Set max lcg value to subnet count
	//fmt.Println(network.IP.String())
	lcg.Init(int64(subnetCount), int64(n))

	// Increment each subnet by the size of one subnet block
	subnetIncrement := big.NewInt(1)
	subnetIncrement.Lsh(subnetIncrement, uint(128-targetPrefixSize))

	return &subnetGenerator{
		baseIP:        baseIPInt,
		subnetMask:    subnetMask,
		subnetCount:   subnetCount,
		currentIndex:  0,
		increment:     subnetIncrement,
		numRandBlocks: (128 - targetPrefixSize) / 16,
		n:             n,
		lcg:           lcg,
		done:          false,
		mode:          mode,
	}, nil
}

// Convert IP to a big integer
func ipToBigInt(ip net.IP) *big.Int {
	ip = ip.To16()
	ipInt := big.NewInt(0)
	ipInt.SetBytes(ip)
	return ipInt
}

// Convert big integer back to IP
func bigIntToIP(ipInt *big.Int) net.IP {
	ipBytes := ipInt.Bytes()
	ip := make(net.IP, 16)
	copy(ip[16-len(ipBytes):], ipBytes) // Ensure the correct length for IPv6
	return ip
}

func parseArguments() Arguments {
	args := Arguments{}
	flag.StringVar(&args.PrefixFile, "prefix", "", "Path to prefix file")
	flag.IntVar(&args.TargetPrefixSize, "size", 0, "Target prefix size")
	flag.BoolVar(&args.TestMode, "test-mode", false, "Enable test mode")
	flag.Int64Var(&args.Seed, "seed", 0, "Random seed for LCG")
	flag.IntVar(&args.Limit, "limit", 1000, "Number of values to generate (default 1000 in test mode)")
	flag.Parse()
	return args
}

func main() {
	if len(os.Args) < 4 {
		fmt.Println("[*] Usage: go run sra_generation.go [prefix file] [target prefix size] [set generation mode r (random), n (network; default) or b (both)] [optional: generate at most n addresses per subnet]")
		return
	}

	prefixFile := os.Args[1]
	targetPrefixSize := 0
	fmt.Sscanf(os.Args[2], "%d", &targetPrefixSize)

	genMode := 'n'
	fmt.Sscanf(os.Args[3], "%c", &genMode)
	switch genMode {
	case 'n':
	case 'r':
	case 'b':
	default:
		fmt.Println("Generation mode only allows r,n, or b")
		fmt.Println("[*] Usage: go run sra_generation.go [prefix file] [target prefix size] [set generation mode r (random), n (network; default) or b (both)] [optional: generate at most n addresses per subnet    ]")
		return
	}
	// Limit number of generated addresses per subnet to n
	n := 0
	if len(os.Args) == 5 {
		fmt.Sscanf(os.Args[4], "%d", &n)
	}
	file, err := os.Open(prefixFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %s\n", err)
		return
	}
	defer file.Close()

	// Read all prefixes into a list
	scanner := bufio.NewScanner(file)
	var generators []*subnetGenerator
	for scanner.Scan() {
		prefix := strings.TrimSpace(scanner.Text())
		gen, err := createSubnetGenerator(prefix, targetPrefixSize, n, genMode)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating subnet generator: %s\n", err)
			return
		}
		generators = append(generators, gen)
	}

	if len(generators) == 0 {
		fmt.Println("No valid prefixes provided.")
		return
	}

	// Channel to pass IP addresses
	addressChan := make(chan string)
	var wg sync.WaitGroup

	// Goroutine to cycle through the generators and send one subnet at a time
	wg.Add(1)
	go func() {
		defer wg.Done()
		activeGenerators := len(generators)
		for activeGenerators > 0 {
			for _, gen := range generators {
				if gen.done {
					continue
				}
				subnet := gen.nextSubnet()
				if subnet != "" {
					addressChan <- subnet
				}
				if gen.done {
					activeGenerators--
				}
			}
		}
		close(addressChan)
	}()

	// Print all addresses from the channel
	wg.Add(1)
	go func() {
		defer wg.Done()
		for address := range addressChan {
			fmt.Println(address)
		}
	}()

	// Wait for all subnets to be processed and printed
	wg.Wait()
}
