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
go run address-generator-ipv6.go [prefix-file] [target prefix size] [set generation mode r (random), n (network; default) or b (both)] [optional: generate at most n addresses per subnet]
```
## Example
Use the `input-prefixes` file as input and generate /48 addresses
```
go run address-generator-ipv6.go input-prefixes 48 b 1
```
### Example output
```
2001:0:b2a7::
2001:0:b2a7:0:422a:71e5:2d19:7c73
2001:2::
2001:2::3337:dfec:7187:87c
2001:4:112::
2001:4:112:0:5ede:1a05:a79d:7d96
2001:200:469b::
2001:200:469b:0:b:ffab:728e:4c2e
2001:200:650::
2001:200:650:0:6780:afd3:e8ea:b78
2001:200:93a::
2001:200:93a:0:18c2:90d4:741:2bb
2001:200:e76::
2001:200:e76:0:f3b3:82ca:a248:2b8c
2001:200:cbff::
2001:200:cbff:0:3e6a:ed1c:ea87:e8c6
2001:200:e393::
2001:200:e393:0:78d8:d9f:3faf:ff2
2001:218:36ef::
2001:218:36ef:0:2d7d:755f:e174:7fd8
```

## Update Version 2
- Using a subnet generator struct to keep track of status
- Cycle through one subnet at a time to create reproducible results
    - as long as the input does not change, the output remains in the same order
## Update Version 3
- Added pseudo-random generator (LCG) to traverse address space in a pseudo-random, reproducible fashion.
- Added subnet generation limit
    - only generate at most n addresses per given prefix
## Update Version 4
- Added new address generation mode
    - generate the subnet-router anycast/network address (n), a random address for the given prefix (r), or both (b)

