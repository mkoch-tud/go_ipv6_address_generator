# Go IPv6 Address Generation Tool
Pass an input file with IPv6 prefixes and a target network size, the tool generates all possible subnets between (and including) the initial prefix and the target prefix size.
If the target prefix size is smaller than the given prefix, the tool generates the supernet with the target prefix size.
The tool can generate random addresses to probe a subnet and can also generate so-called subnet-router anycast addresses (see [RFC-1884](https://datatracker.ietf.org/doc/html/rfc1884) for reference)

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

## Structure of Blocklist File
```
# IPv6 Blocklist file
2001:0:469e:0:aad9:5c1c:1b08:879e

2001:200:90a7::/48 # Testing to block prefixes
2001:200:eb99:100::/64
```

## Example config file structure
```
{
  "PrefixFile": "../input/input-prefixes",
  "TargetSubnetSize": 48,
  "Mode": "b",
  "Seed": 133371717287384832,
  "PerPrefixLimit": 1,
  "TotalLimit": 100,
  "BlocklistFile": "config/blocklist.conf"
}
```

## Configuration Options
| Command Line Argument           | Config File Key     | Description                                          | Required | Default Value |
|---------------------------------|---------------------|------------------------------------------------------|----------|--------------|
| `--prefix-file <file>`         | `"PrefixFile"`      | Path to the input prefix file                        | Yes      | N/A          |
| `--target-subnet-size <size>`  | `"TargetSubnetSize"`| Size of the generated target subnets (e.g., 48 for /48) | No      | 48          |
| `--mode <n, r, b>`            | `"Mode"`           | Mode selection: `n`, `r`, or `b`         | No       | `n`          |
| `--limit-per-prefix <int>`    | `"PerPrefixLimit"`  | Max number of addresses per given prefix, `-1` disables limit | No       | `-1`         |
| `--total-limit <max>`         | `"TotalLimit"`      | Maximum number of generated addresses, `-1` disables limit | No       | `-1`         |
| `--seed <seed>`               | `"Seed"`           | Seed value for the linear congruential generator   | No       | 1337371717283484832 |
| `--blocklist-file <file>`               | `"BlocklistFile"`           | Path to blocklist file   | No       | configs/blocklist.conf |

## Usage
```
go run sra_generation_cyclic-v4.go --prefix-file <file> --target-subnet-size <target size> --mode <n (default), r, b> --limit-per-prefix <int, default -1 (no limit) --total-limit <max. amount of generated addresses> --seed <seed for lcg> --blocklist-file <file>
```
or
```
go run sra_generation_cyclic-v4.go --config-file <file>
```
## Example
Use the `input-prefixes` file as input and generate /48 addresses
```
go run address-generator-ipv6.go --prefix-file input/input-prefixes --target-subnet-size 48 --mode b --limit-per-prefix 1 --total-limit 100
```
or
```
go run address-generator-ipv6.go --config-file config/config-go-tool 
```
### Example output
```
2001:0:469e::
2001:2::
Skipping blocked addresses for prefix 2001:200:90a7::/48: 2001:200:90a7:: and 2001:200:90a7:0:ab6e:7096:e335:5176
2001:4:112::
2001:200:286a::
2001:200:6fd::
2001:200:60c3::
2001:200:97d::
2001:200:eba::
2001:200:d86f::
2001:200:eb99::
2001:218:a39f::
2001:0:e1a7::
2001:200:c02d::
2001:200:6de::
2001:200:60c6::
2001:200:978::
2001:200:ec1::
2001:200:d974::
2001:200:e3ec::
2001:218:5aa::
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
## Update Version 5
- Fixed random address generator to also use static seed
    - same input provides same output --> reproducibility
- Added go flags to parse arguments
- Added option for config file in json format to ease generator setup
- Added total-limit option, algorithm will stop after generating [X] addresses
## Update Version 6
- Fixed random subnet generator to use a static seed for reproducible results
- Implemented blocklist feature
    - Add --blocklist-file argument
    - Make ZMap blocklist structure work
- Printing informational log messages to stderr
