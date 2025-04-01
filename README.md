# Go IPv6 Address Generation Tool
Pass an input file with IPv6 prefixes and a target network size, the tool generates all possible subnets between (and including) the initial prefix and the target prefix size.
If the target prefix size is smaller than the given prefix, the tool generates the supernet with the target prefix size.
Currently, the tool sets all remaining bits to zero, resulting in the so-called subnet-router anycast address (see [RFC-1884](https://datatracker.ietf.org/doc/html/rfc1884) for reference)

## Structure of Prefix File
One Prefix per line.
```
2001::/32
2001:2::/48
2001:4:112::/48
2001:200::/32
2001:200:600::/40
2001:200:900::/40
2001:200:e00::/40
2001:200:c000::/35
2001:200:e000::/35
2001:218::/32
```

## Usage
```
go run address-generator-ipv6.go [prefix-file] [target prefix size] 
```
## Example
Use the `input-prefixes` file as input and generate /64 addresses
```
go run address-generator-ipv6.go input-prefixes 64
```
### Example output
```
2001:218:fff6::
2001:218:fff7::
2001:218:fff8::
2001:218:fff9::
2001:218:fffa::
2001:218:fffb::
```

## Update Version 2
- Using a subnet generator struct to keep track of status
- Cycle through one subnet at a time to create reproducible results
    - as long as the input does not change, the output remains in the same order
