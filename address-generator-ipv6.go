package main

import (
	"bufio"
	"encoding/json"
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


// Read blocklist file and parse IPv6 prefixes to exclude them from scans
func readBlocklist(filename string) ([]*net.IPNet, error) {
    file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var prefixes []*net.IPNet
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines or comment lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Remove inline comments
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		// Ensure it's a valid IPv6 CIDR or address
		if !strings.Contains(line, "/") {
			line += "/128" // Convert single address to CIDR notation
		}
		_, ipNet, err := net.ParseCIDR(line)
		if err != nil {
			return nil, fmt.Errorf("invalid IPv6 prefix: %s", line)
		}
		prefixes = append(prefixes, ipNet)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return prefixes, nil
}

// Simple prng to generate random 16-bit IPv6-blocks
type Prng16Bit struct {
	prng  *rand.Rand
	value uint16
}

func (prng *Prng16Bit) Init(seed int64) {
	prng.prng = rand.New(rand.NewSource(seed))
}

func (prng *Prng16Bit) Next() uint16 {
	prng.value = uint16(prng.prng.Intn(1 << 16))
	return prng.value
}

// Linear Congruential Generator
// as described in https://stackoverflow.com/a/53551417
type Lcg struct {
	value      int64
	offset     int64
	multiplier int64
	modulus    int64
	max        int64
	max_iter   int64
	found      int64
    prng       *rand.Rand
}

func (lcg *Lcg) Init(seed int64, max_count int64, stop int64) {
	// Seed range with a random integer.
	//fmt.Println("%d",max_count)
	lcg.prng = rand.New(rand.NewSource(seed))
	lcg.value = lcg.prng.Int63n(max_count)
	lcg.offset = lcg.prng.Int63n(max_count)*2 + 1                                  // Pick a random odd-valued offset.
    //fmt.Println("%d",lcg.value)
    //fmt.Println("%d",lcg.offset)
    //fmt.Println("%d",max_count)
	lcg.multiplier = 4*(max_count/4) + 1                                       // Pick a multiplier 1 greater than a multiple of 4
	lcg.modulus = int64(math.Pow(2, math.Ceil(math.Log2(float64(max_count))))) // Pick a modulus just big enough to generate all numbers (power of 2)
	lcg.found = 0                                                              // Track how many random numbers have been returned
	lcg.max = max_count
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
	subnetCount   int64
	currentIndex  int64
	increment     *big.Int
	numRandBlocks int
	n             int64
	lcg           Lcg
	prng          Prng16Bit
	done          bool
	mode          string
    hasBlockedIPs bool
    blocklist     []*net.IPNet
}

// Checks if a given prefix is within any blocked subnets
func isBlocked(ipNet *net.IPNet, blocklist []*net.IPNet) bool {
	for _, blocked := range blocklist {
		if blocked.Contains(ipNet.IP) {
			return true
		}
	}
	return false
}

func IncrementLastNBlocks(num *big.Int, numBlocks int, prng Prng16Bit) *big.Int {
	// Mask and shift values to isolate each 16-bit block
	var mask uint64 = 0xFFFF
	newNum := new(big.Int).Set(num) // Make a copy to modify
	oldNum := new(big.Int).Set(num) // Make a copy to modify

	for i := 0; i < numBlocks; i++ {
		// Isolate the 16-bit block
		shift := uint(i * 16)
		block := uint64(oldNum.Rsh(oldNum, shift).Uint64() & mask)

		// Generate a random increment and add to the block (mod 2^16)
		increment := uint64(prng.Next())
		newBlock := (block + increment) & mask

		newNum.Or(newNum, new(big.Int).SetUint64(newBlock<<shift))
	}
	//fmt.Println("New Increment")
	//fmt.Println(newNum)
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

	subnetIPInt := new(big.Int).Add(gen.baseIP, new(big.Int).Mul(gen.increment, big.NewInt(nextValue)))
	subnetIPRandInt := IncrementLastNBlocks(subnetIPInt, gen.numRandBlocks, gen.prng)

    subnetIP := bigIntToIP(subnetIPInt).Mask(gen.subnetMask)
    subnetIPRand := bigIntToIP(subnetIPRandInt)
	//fmt.Println(bigIntToIP(gen.baseIP))
	//fmt.Println(subnetIP.String())
	//fmt.Println(subnetIPRand.String())
	//fmt.Println("----")

    if gen.hasBlockedIPs {
		for _,blocked := range gen.blocklist {
			if (blocked.Contains(subnetIP) && (gen.mode=="n" || gen.mode=="b")) ||
            (blocked.Contains(subnetIPRand) && (gen.mode=="r" || gen.mode=="b")){
				fmt.Fprintf(os.Stderr, "Skipping blocked addresses for prefix %s: %s and %s\n", blocked.String(),subnetIP, subnetIPRand)
	            //fmt.Println(bigIntToIP(gen.baseIP))
                //fmt.Println("---")
                return gen.nextSubnet() // Recursively get the next valid address
			}
		}
    }
    
	gen.currentIndex++
    //fmt.Println("+++")
	//fmt.Println(bigIntToIP(gen.baseIP))
    //fmt.Println(subnetIP.String())
    //fmt.Println(subnetIPRand.String())
    //fmt.Println("+++")

	switch gen.mode {
	case "n":
		return subnetIP.String()
	case "r":
		return subnetIPRand.String()
	default:
		return subnetIP.String() + "\n" + subnetIPRand.String()
	}
}

// Create a subnet generator for a given prefix
func createSubnetGenerator(prefix string, targetPrefixSize int, n int64, mode string, blocklist []*net.IPNet) (*subnetGenerator, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, fmt.Errorf("Invalid prefix: %s", prefix)
	}
    // Skip generator if prefix is subnet of blocked network
    if isBlocked(network, blocklist){
        fmt.Fprintf(os.Stderr, "Skipping blocked prefix: %s\n", prefix)
        return nil,nil
    }

	// Check if this prefix is a supernet of any blocked prefixes
    // if so, add them to the blocklist of the generator and indicate
    // that the generator contains blocklisted networks/IPs
	var relevantBlocklist []*net.IPNet
	hasBlockedIPs := false
	for _, blocked := range blocklist {
		if network.Contains(blocked.IP) {
			relevantBlocklist = append(relevantBlocklist, blocked)
			hasBlockedIPs = true
		}
	}

	networkPrefixSize, _ := network.Mask.Size()

	var lcg Lcg
	//Init 16 bit prng for traversing random subnets
	var prng Prng16Bit
	var prngSeed int64
	// needed for initialization of the prng
	// we use the given ip prefix as a start seed to generate the subnets
	// advantage: seed is unique per prefix but also good for reproducible results
	var tmpInt big.Int

	if networkPrefixSize > targetPrefixSize {
		// If the input prefix size is larger than the target, calculate the supernet (larger than the original)
		supernetMask := net.CIDRMask(targetPrefixSize, 128)
		supernet := &net.IPNet{
			IP:   network.IP.Mask(supernetMask),
			Mask: supernetMask,
		}
		baseIPInt := ipToBigInt(supernet.IP)
		prngSeed = tmpInt.Rsh(baseIPInt, 64).Int64()
		prng.Init(prngSeed)
		return &subnetGenerator{
			baseIP:        baseIPInt,
			subnetMask:    supernet.Mask,
			subnetCount:   1, // Just one supernet
			currentIndex:  0,
			increment:     big.NewInt(0), // No increment needed
			numRandBlocks: (128 - targetPrefixSize) / 16,
			n:             n,
			lcg:           lcg,
			prng:          prng,
			done:          false,
			mode:          mode,
            hasBlockedIPs: hasBlockedIPs,
            blocklist:     relevantBlocklist,
		}, nil
	}

	// Subnet calculation for targetPrefixSize
	subnetMask := net.CIDRMask(targetPrefixSize, 128)
	baseIPInt := ipToBigInt(network.IP)

	//fmt.Println(baseIPInt)
	//fmt.Println(tmpInt.Rsh(baseIPInt, 64))
	prngSeed = tmpInt.Rsh(baseIPInt, 64).Int64()
	prng.Init(prngSeed)
	// Number of subnets we need to generate
	subnetCount := int64(1 << (targetPrefixSize - networkPrefixSize))
	// Set max lcg value to subnet count -- deprecated
	//fmt.Println(network.IP.String())
	lcg.Init(prngSeed, subnetCount, n)

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
		prng:          prng,
		done:          false,
		mode:          mode,
		hasBlockedIPs: hasBlockedIPs,
		blocklist:     relevantBlocklist,
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

type Arguments struct {
	ConfigFile       string
	Mode             string
	PrefixFile       string
	TargetSubnetSize int
	PerPrefixLimit   int64
	TotalLimit       int64
	Seed             int64
    BlocklistFile    string
}

func parseArguments() Arguments {
	args := Arguments{}

	flag.StringVar(&args.ConfigFile, "config-file", "", "Path to config file")
	flag.StringVar(&args.PrefixFile, "prefix-file", "", "Path to input prefix file")
	flag.IntVar(&args.TargetSubnetSize, "target-subnet-size", 48, "Target size of generated subnets (default 48)")
	flag.StringVar(&args.Mode, "mode", "n", "Operating mode -- generate network (n, default), random (r), or both (b) addresses")
	flag.Int64Var(&args.Seed, "seed", 1337371717283484832, "Random seed for LCG (optional, default is 1337371717283484832)")
	flag.Int64Var(&args.PerPrefixLimit, "limit-per-prefix", -1, "Limit number of generated subnets per prefix (-1 -> no limit, default)")
	flag.Int64Var(&args.TotalLimit, "total-limit", -1, "Limit number of generated subnets globally (-1 -> no limit, default)")
    flag.StringVar(&args.BlocklistFile, "blocklist-file","config/blocklist.conf","Path to blocklist file")
	flag.Parse()

	if args.ConfigFile != "" {
		file, err := os.Open(args.ConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening config file: %s\n", err)
			os.Exit(1)
		}
		defer file.Close()

		decoder := json.NewDecoder(file)
		//configArgs := Arguments{}
		if err := decoder.Decode(&args); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing config file: %s\n", err)
			os.Exit(1)
		}
		//fmt.Println(args)
	}

	return args
}

func main() {
	args := parseArguments()

	if args.PrefixFile == "" {
		fmt.Println("[*] No input file in CIDR prefix notation provided. (--prefix-file/Prefix-File)")
		fmt.Println("Usage: go run sra_generation_cyclic-v4.go --prefix-file <file> --target-subnet-size <target size> --mode <n (default), r, b> --limit-per-prefix <int, default -1 (no limit) --total-limit <max. amount of generated subnets> --seed <seed for lcg> --blocklist-file <path to blocklist file>")
		fmt.Println("   Or: go run sra_generation_cyclic-v4.go --config-file <file>")
		return
	}

	var blocklist []*net.IPNet
	if args.BlocklistFile != "" {
		var err error
		blocklist, err = readBlocklist(args.BlocklistFile)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	file, err := os.Open(args.PrefixFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening file: %s\n", err)
		return
	}
	defer file.Close()

	// initialize random generator with static seed
	rand.Seed(args.Seed)

	// Read all prefixes into a list
	scanner := bufio.NewScanner(file)
	var generators []*subnetGenerator
	for scanner.Scan() {
		prefix := strings.TrimSpace(scanner.Text())
		gen, err := createSubnetGenerator(prefix, args.TargetSubnetSize, args.PerPrefixLimit, args.Mode, blocklist)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating subnet generator: %s\n", err)
			return
		}
        if gen == nil {
            //fmt.Fprintf(os.Stderr, "Skipping prefix %s (has supernet on blocklist)\n",prefix)
            continue
        }
		generators = append(generators, gen)
	}

	if len(generators) == 0 {
		fmt.Fprintf(os.Stderr, "No valid prefixes provided.")
		return
	}
	//fmt.Println(len(generators))
	// Channel to pass IP addresses
	addressChan := make(chan string)
	var wg sync.WaitGroup

	// Goroutine to cycle through the generators and send one subnet at a time
	wg.Add(1)
	go func() {
		defer wg.Done()
		activeGenerators := len(generators)
		// needs to be float to be able to compare with math.Inf
		generatedSubnets := float64(0)
		var maxGeneratedSubnets float64
		if args.TotalLimit == -1 {
			maxGeneratedSubnets = math.Inf(1)
		} else {
			maxGeneratedSubnets = float64(args.TotalLimit)
		}
		//fmt.Println(maxGeneratedSubnets)
		//fmt.Println(generatedSubnets)
		for activeGenerators > 0 && generatedSubnets < maxGeneratedSubnets {
			for _, gen := range generators {
				if generatedSubnets >= (maxGeneratedSubnets) {
					//fmt.Println("Done generating subnets.")
					break
				}
				if gen.done {
					continue
				}
				subnet := gen.nextSubnet()
				if subnet != "" {
					addressChan <- subnet
					generatedSubnets++
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
