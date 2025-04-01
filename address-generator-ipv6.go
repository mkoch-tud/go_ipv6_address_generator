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

func generateSubnets(prefix string, targetPrefixSize int, wg *sync.WaitGroup, out chan<- string) {
	defer wg.Done()

	_, network, err := net.ParseCIDR(prefix)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid prefix: %s\n", prefix)
		return
	}

	networkPrefixSize, _ := network.Mask.Size()
	if networkPrefixSize == targetPrefixSize {
		out <- network.IP.String() //+ "/" + fmt.Sprint(targetPrefixSize)
		return
	}
	if networkPrefixSize > targetPrefixSize {
		// If the prefix size is larger than the target, just use the supernet
		supernetMask := net.CIDRMask(targetPrefixSize, 128)
		supernet := &net.IPNet{
			IP:   network.IP.Mask(supernetMask),
			Mask: supernetMask,
		}
		out <- supernet.IP.String() //+ "/" + fmt.Sprint(targetPrefixSize)
		return
	}

	// Subnet calculation for targetPrefixSize
	subnetMask := net.CIDRMask(targetPrefixSize, 128)

	// Convert network IP to a big integer to calculate subnets
	baseIPInt := ipToBigInt(network.IP)

	// Number of subnets we need to generate
	subnetCount := 1 << (targetPrefixSize - networkPrefixSize)

	// Increment each subnet by the size of one subnet block
	subnetIncrement := big.NewInt(1)
	subnetIncrement.Lsh(subnetIncrement, uint(128-targetPrefixSize)) // 128 - targetPrefixSize gives the number of hosts per subnet

	// Generate each subnet
	for i := 0; i < subnetCount; i++ {
		subnetIP := new(big.Int).Add(baseIPInt, new(big.Int).Mul(subnetIncrement, big.NewInt(int64(i))))
		subnet := &net.IPNet{
			IP:   bigIntToIP(subnetIP).Mask(subnetMask),
			Mask: subnetMask,
		}
		out <- subnet.IP.String() //+ "/" + fmt.Sprint(targetPrefixSize)
	}
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

	// Channel to pass IP addresses
	addressChan := make(chan string)
	var wg sync.WaitGroup

	// Start reading prefixes and generating subnets
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		prefix := strings.TrimSpace(scanner.Text())
		wg.Add(1)
		go generateSubnets(prefix, targetPrefixSize, &wg, addressChan)
	}

	go func() {
		wg.Wait()
		close(addressChan)
	}()

	// Print all addresses from the channel
	for address := range addressChan {
		fmt.Println(address)
	}
}

