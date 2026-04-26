// Command dimension resizes GCE instances by core count.
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

var instances = map[string]instance{
	"penny": {"the-good-news", "us-east4-a"},
}

type instance struct {
	project string
	zone    string
}

type size struct {
	typ string
	ram string
}

var kSizes = map[string]size{
	"0":  {"e2-micro", "1 GB"},         // ~$6/month
	"2":  {"c3-highcpu-4", "8 GB"},     // ~$131/month
	"4":  {"c3-highcpu-8", "16 GB"},    // ~$263/month
	"11": {"c3-highcpu-22", "44 GB"},   // ~$723/month
	"22": {"c3-highcpu-44", "88 GB"},   // ~$1446/month
	"44": {"c3-highcpu-88", "176 GB"},  // ~$2891/month
	"88": {"c3-highcpu-176", "352 GB"}, // ~$5782/month
}

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "usage: dimension <name> <cores>\n")
		fmt.Fprintf(os.Stderr, "       cores: 0, 2, 4, 11, 22, 44, 88\n")
		names := make([]string, 0, len(instances))
		for k := range instances {
			names = append(names, k)
		}
		fmt.Fprintf(os.Stderr, "       names: %s\n", strings.Join(names, ", "))
		os.Exit(1)
	}
	name := os.Args[1]
	cores := os.Args[2]
	inst, ok := instances[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: unknown instance %q\n", name)
		os.Exit(1)
	}
	s, ok := kSizes[cores]
	if !ok {
		fmt.Fprintf(os.Stderr, "error: invalid core count %q\n", cores)
		fmt.Fprintf(os.Stderr, "       valid: 0, 2, 4, 11, 22, 44, 88\n")
		os.Exit(1)
	}

	current := gcloud("compute", "instances", "describe", name,
		"--zone", inst.zone,
		"--project", inst.project,
		"--format", "value(machineType.basename())")
	if current == s.typ {
		fmt.Printf("%s: already %s (%s cores, %s)\n", name, s.typ, cores, s.ram)
		return
	}
	fmt.Printf("%s: %s → %s (%s cores, %s)\n", name, current, s.typ, cores, s.ram)

	fmt.Printf("stopping %s...\n", name)
	gcloud("compute", "instances", "stop", name,
		"--zone", inst.zone,
		"--project", inst.project)
	fmt.Println("stopped")

	gcloud("compute", "instances", "set-machine-type", name,
		"--zone", inst.zone,
		"--project", inst.project,
		"--machine-type", s.typ)

	fmt.Printf("starting %s...\n", name)
	gcloud("compute", "instances", "start", name,
		"--zone", inst.zone,
		"--project", inst.project)

	ip := gcloud("compute", "instances", "describe", name,
		"--zone", inst.zone,
		"--project", inst.project,
		"--format", "value(networkInterfaces[0].accessConfigs[0].natIP)")
	fmt.Printf("%s is up at %s\n", name, ip)
}

func gcloud(args ...string) string {
	cmd := exec.Command("gcloud", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "gcloud %s failed: %v\n", strings.Join(args[:3], " "), err)
		os.Exit(1)
	}
	return strings.TrimSpace(string(out))
}
