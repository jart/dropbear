// Command dimension resizes EC2 instances by core count.
//
// Usage:
//
//	go run ./cmd/dimension penny 4
package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

var instances = map[string]string{
	"penny": "i-04790977c160f6adb",
}

type size struct {
	typ string
	ram string
}

var z1dSizes = map[string]size{
	"0":  {"t3a.micro", "1 GB"},      // $7/month
	"1":  {"z1d.large", "16 GB"},     // $70/month
	"2":  {"z1d.xlarge", "32 GB"},    // $140/month
	"4":  {"z1d.2xlarge", "64 GB"},   // $402/month
	"6":  {"z1d.3xlarge", "96 GB"},   // $804/month
	"12": {"z1d.6xlarge", "192 GB"},  // $1607/month
	"24": {"z1d.12xlarge", "384 GB"}, // $3214/month
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: dimension <name> <cores>\n")
		fmt.Fprintf(os.Stderr, "       cores: 0, 1, 2, 4, 6, 12, 24\n")
		names := make([]string, 0, len(instances))
		for k := range instances {
			names = append(names, k)
		}
		fmt.Fprintf(os.Stderr, "       names: %s\n", strings.Join(names, ", "))
		os.Exit(1)
	}
	name := os.Args[1]
	cores := os.Args[2]
	instanceID, ok := instances[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown instance %q\n", name)
		os.Exit(1)
	}
	s, ok := z1dSizes[cores]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: invalid core count %q\n", cores)
		fmt.Fprintf(os.Stderr, "       valid: 1, 2, 4, 6, 12, 24\n")
		os.Exit(1)
	}

	current := aws("ec2", "describe-instances",
		"--instance-ids", instanceID,
		"--query", "Reservations[0].Instances[0].InstanceType",
		"--output", "text")
	if current == s.typ {
		fmt.Printf("%s: already %s (%s cores, %s)\n", name, s.typ, cores, s.ram)
		return
	}
	fmt.Printf("%s: %s → %s (%s cores, %s)\n", name, current, s.typ, cores, s.ram)

	fmt.Printf("stopping %s...\n", name)
	aws("ec2", "stop-instances", "--instance-ids", instanceID, "--output", "text")
	aws("ec2", "wait", "instance-stopped", "--instance-ids", instanceID)
	fmt.Println("stopped")

	aws("ec2", "modify-instance-attribute",
		"--instance-id", instanceID,
		"--instance-type", s.typ)

	fmt.Printf("starting %s...\n", name)
	aws("ec2", "start-instances", "--instance-ids", instanceID, "--output", "text")
	aws("ec2", "wait", "instance-running", "--instance-ids", instanceID)

	ip := aws("ec2", "describe-instances",
		"--instance-ids", instanceID,
		"--query", "Reservations[0].Instances[0].PublicIpAddress",
		"--output", "text")
	fmt.Printf("%s is up at %s\n", name, ip)
}

func aws(args ...string) string {
	cmd := exec.Command("aws", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "aws %s failed: %v\n", args[0]+" "+args[1], err)
		os.Exit(1)
	}
	return strings.TrimSpace(string(out))
}
