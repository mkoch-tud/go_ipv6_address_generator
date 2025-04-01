package main

import (
	"bufio"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
)

// Struct to hold subnet generation state for each prefix
type subnetGenerator struct {
	baseIP       *big.Int
	subnetMask   net.IPMask
	subnetCount  int
	currentIndex int
	increment    *big.Int
	done         bool
}

// Generate the next subnet address for this prefix
func (gen *subnetGenerator) nextSubnet() string {
	if gen.currentIndex >= gen.subnetCount {
		gen.done = true // Mark generator as done
		return ""       // No more subnets to generate
	}

	// Calculate the subnet IP
	subnetIP := new(big.Int).Add(gen.baseIP, new(big.Int).Mul(gen.increment, big.NewInt(int64(gen.currentIndex))))
	subnet := &net.IPNet{
		IP:   bigIntToIP(subnetIP).Mask(gen.subnetMask),
		Mask: gen.subnetMask,
	}
	gen.currentIndex++
	return subnet.IP.String() //+ "/" + fmt.Sprint(len(gen.subnetMask) * 8)
}

// Create a subnet generator for a given prefix
func createSubnetGenerator(prefix string, targetPrefixSize int) (*subnetGenerator, error) {
	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		return nil, fmt.Errorf("Invalid prefix: %s", prefix)
	}

	networkPrefixSize, _ := network.Mask.Size()

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
			done:         false,
		}, nil
	}

	// Subnet calculation for targetPrefixSize
	subnetMask := net.CIDRMask(targetPrefixSize, 128)
	baseIPInt := ipToBigInt(network.IP)

	// Number of subnets we need to generate
	subnetCount := 1 << (targetPrefixSize - networkPrefixSize)

	// Increment each subnet by the size of one subnet block
	subnetIncrement := big.NewInt(1)
	subnetIncrement.Lsh(subnetIncrement, uint(128-targetPrefixSize))

	return &subnetGenerator{
		baseIP:       baseIPInt,
		subnetMask:   subnetMask,
		subnetCount:  subnetCount,
		currentIndex: 0,
		increment:    subnetIncrement,
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
		fmt.Println("[*] Usage: go run sra_generation.go [prefix file] [target prefix size]")
		return
	}

	// Reading arguments
	prefixFile := os.Args[1]
	targetPrefixSize := 0
	fmt.Sscanf(os.Args[2], "%d", &targetPrefixSize)

	// Open prefix file
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
		gen, err := createSubnetGenerator(prefix, targetPrefixSize)
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

