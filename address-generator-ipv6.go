package main

import (
	"bufio"
	"fmt"
	"math/big"
    "math/rand"
    "math"
	"net"
	"os"
	"strings"
	"sync"
)

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
	lcg.multiplier = 4*(seed/4) + 1                                // Pick a multiplier 1 greater than a multiple of 4
	lcg.modulus = int64(math.Pow(2, math.Ceil(math.Log2(float64(seed))))) // Pick a modulus just big enough to generate all numbers (power of 2)
	lcg.found = 0                                                       // Track how many random numbers have been returned
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
	baseIP       *big.Int
	subnetMask   net.IPMask
	subnetCount  int
	currentIndex int
	increment    *big.Int
    n            int
    lcg          Lcg
	done         bool
}

// Generate the next subnet address for this prefix
func (gen *subnetGenerator) nextSubnet() string {
	if gen.currentIndex >= gen.subnetCount || (gen.n>0 && gen.currentIndex >= gen.n){
		gen.done = true // Mark generator as done
		return ""       // No more subnets to generate
	}
    var nextValue int64

    if gen.lcg.Has_next() {
            nextValue = gen.lcg.Next()
    }else{
        gen.done = true
        return ""
    }

	// Calculate the subnet IP
	subnetIP := new(big.Int).Add(gen.baseIP, new(big.Int).Mul(gen.increment, big.NewInt(nextValue)))
	subnet := &net.IPNet{
		IP:   bigIntToIP(subnetIP).Mask(gen.subnetMask),
		Mask: gen.subnetMask,
	}
	gen.currentIndex++
	return subnet.IP.String() //+ "/" + fmt.Sprint(len(gen.subnetMask) * 8)
}

// Create a subnet generator for a given prefix
func createSubnetGenerator(prefix string, targetPrefixSize int, n int) (*subnetGenerator, error) {
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
			baseIP:       ipToBigInt(supernet.IP),
			subnetMask:   supernet.Mask,
			subnetCount:  1, // Just one supernet
			currentIndex: 0,
			increment:    big.NewInt(0), // No increment needed
            n:            n,
            lcg:          lcg,    
			done:         false,
		}, nil
	}

	// Subnet calculation for targetPrefixSize
	subnetMask := net.CIDRMask(targetPrefixSize, 128)
	baseIPInt := ipToBigInt(network.IP)

	// Number of subnets we need to generate
	subnetCount := 1 << (targetPrefixSize - networkPrefixSize)
    	// Set max lcg value to subnet count
    	//fmt.Println(network.IP.String())
	lcg.Init(int64(subnetCount),int64(n))

	// Increment each subnet by the size of one subnet block
	subnetIncrement := big.NewInt(1)
	subnetIncrement.Lsh(subnetIncrement, uint(128-targetPrefixSize))

	return &subnetGenerator{
		baseIP:       baseIPInt,
		subnetMask:   subnetMask,
		subnetCount:  subnetCount,
		currentIndex: 0,
		increment:    subnetIncrement,
        n:            n,
        lcg:          lcg,
		done:         false,
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

func main() {
	if len(os.Args) < 3 {
		fmt.Println("[*] Usage: go run sra_generation.go [prefix file] [target prefix size] [optional: generate at most n addresses per subnet]")
		return
	}

	prefixFile := os.Args[1]
	targetPrefixSize := 0
	fmt.Sscanf(os.Args[2], "%d", &targetPrefixSize)
    
    // Limit number of generated addresses per subnet to n
    n := 0
    if len(os.Args) == 4 {
        fmt.Sscanf(os.Args[3], "%d", &n)
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
		gen, err := createSubnetGenerator(prefix, targetPrefixSize, n)
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
