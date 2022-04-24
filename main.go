package main

import (
	"flag"
	"fmt"
	gs1200 "gs1200-exporter/internal"
	"os"
)

var (
	Version = "development"

	listenPort = flag.String("port", "9934",
		"Port on which to expose metrics.")
	gs1200Address = flag.String("address", "192.168.1.3",
		"IP address or hostname of the GS1200")
	gs1200Password = flag.String("password", "********",
		"Password to log on to the GS1200")
	versionFlag = flag.Bool("version", false,
		"Show gs1200-exporter version")
)

func main() {
	flag.Parse()
	if *versionFlag {
		fmt.Println("gs1200-exporter", Version)
		os.Exit(0)
	}
	collector, err := gs1200.GS1200Collector(
		getEnv("GS1200_ADDRESS", *gs1200Address),
		getEnv("GS1200_PASSWORD", *gs1200Password),
	)
	if err != nil {
		fmt.Println("Cannot start collector:", err)
		return
	}
	exporter := gs1200.GS1200Exporter(*collector, *listenPort)
	exporter.Run()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
