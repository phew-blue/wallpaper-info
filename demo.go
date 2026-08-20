package main

// DemoInfo returns fixed, synthetic system facts for documentation screenshots and tests.
// Every address is from a documentation or private range so a published preview can never
// leak a real machine's identity or WAN address.
func DemoInfo() Info {
	return Info{
		User:     "demo",
		Host:     "DEMO-PC",
		OS:       "Windows 11 Pro (25H2)",
		Uptime:   "7h 8m",
		CPU:      "Intel Core i5-8265U · 8 cores",
		RAM:      "16 GiB RAM",
		Disk:     "C:  931 GiB · 86% free",
		PublicIP: "203.0.113.42", // RFC 5737 documentation range
		Nics: []NIC{
			{Name: "Ethernet", IP: "192.168.1.20"},
			{Name: "WiFi", IP: "192.168.1.21"},
		},
	}
}
