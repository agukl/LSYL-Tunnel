package version

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	AppVersion          = "2.0.1"
	LegacyClientVersion = "1.1.0"

	ClientConfigVersion               = 1
	ClientConfigRequiresClientVersion = "1.1.0"
	ServerConfigVersion               = 1
	ServerConfigRequiresServerVersion = "2.0.0"

	ProtocolVersion            = 2
	LegacyProtocolVersion      = 1
	MinCompatibleClientVersion = "2.0.0"
)

func Compare(a, b string) (int, error) {
	left, err := parse(a)
	if err != nil {
		return 0, err
	}
	right, err := parse(b)
	if err != nil {
		return 0, err
	}
	for i := range left {
		switch {
		case left[i] < right[i]:
			return -1, nil
		case left[i] > right[i]:
			return 1, nil
		}
	}
	return 0, nil
}

func Less(a, b string) (bool, error) {
	cmp, err := Compare(a, b)
	return cmp < 0, err
}

func Greater(a, b string) (bool, error) {
	cmp, err := Compare(a, b)
	return cmp > 0, err
}

func Validate(v string) error {
	_, err := parse(v)
	return err
}

func CheckMin(current, minimum, subject string) error {
	minimum = strings.TrimSpace(minimum)
	if minimum == "" {
		return nil
	}
	less, err := Less(current, minimum)
	if err != nil {
		return fmt.Errorf("%s requires invalid minimum version %q: %w", subject, minimum, err)
	}
	if less {
		return fmt.Errorf("%s requires version >= %s, current version is %s", subject, minimum, current)
	}
	return nil
}

func CheckMax(current, maximum, subject string) error {
	maximum = strings.TrimSpace(maximum)
	if maximum == "" {
		return nil
	}
	greater, err := Greater(current, maximum)
	if err != nil {
		return fmt.Errorf("%s requires invalid maximum version %q: %w", subject, maximum, err)
	}
	if greater {
		return fmt.Errorf("%s requires version <= %s, current version is %s", subject, maximum, current)
	}
	return nil
}

func parse(v string) ([3]int, error) {
	var out [3]int
	v = strings.TrimSpace(strings.TrimPrefix(v, "v"))
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, fmt.Errorf("version must have three parts")
	}
	for i, part := range parts {
		if part == "" {
			return out, fmt.Errorf("version part %d is empty", i+1)
		}
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return out, fmt.Errorf("version part %d is invalid", i+1)
		}
		out[i] = n
	}
	return out, nil
}
