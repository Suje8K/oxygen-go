package sqlutil

import (
	"fmt"
	"net"
	"os"
	"strings"
)

type TestDB struct {
	DriverName string
	ConnStr    string
}

type NetworkAddress struct {
	Host string
	Port string
}

func PostgresTestDB() TestDB {
	host := os.Getenv("POSTGRES_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("POSTGRES_PORT")
	if port == "" {
		port = "5432"
	}
	connStr := fmt.Sprintf("user=oxygenuser password=oxygenpass host=%s port=%s dbname=oxygendb sslmode=disable",
		host, port)
	return TestDB{
		DriverName: "postgres",
		ConnStr:    connStr,
	}
}

func SplitHostPortDefault(input, defaultHost, defaultPort string) (NetworkAddress, error) {
	addr := NetworkAddress{
		Host: defaultHost,
		Port: defaultPort,
	}
	if len(input) == 0 {
		return addr, nil
	}

	start := 0
	// Determine if IPv6 address, in which case IP address will be enclosed in square brackets
	if strings.Index(input, "[") == 0 {
		addrEnd := strings.LastIndex(input, "]")
		if addrEnd < 0 {
			// Malformed address
			return addr, fmt.Errorf("malformed IPv6 address: '%s'", input)
		}

		start = addrEnd
	}
	if strings.LastIndex(input[start:], ":") < 0 {
		// There's no port section of the input
		// It's still useful to call net.SplitHostPort though, since it removes IPv6
		// square brackets from the address
		input = fmt.Sprintf("%s:%s", input, defaultPort)
	}

	host, port, err := net.SplitHostPort(input)
	if err != nil {
		return addr, fmt.Errorf("net.SplitHostPort failed for '%s': %w", input, err)
	}

	if len(host) > 0 {
		addr.Host = host
	}
	if len(port) > 0 {
		addr.Port = port
	}

	return addr, nil
}
