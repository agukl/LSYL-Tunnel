package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func runInteractive(out io.Writer, opts options) error {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "LSYL Tunnel Profile Tool")
		fmt.Fprintf(out, "Install:  %s\n", opts.installDir)
		fmt.Fprintf(out, "Profiles: %s\n", opts.profilesDir)
		if name, ok := activeProfileName(opts); ok {
			fmt.Fprintf(out, "Active:   %s\n", name)
		} else {
			fmt.Fprintln(out, "Active:   <direct install files>")
		}
		fmt.Fprintln(out, "")
		fmt.Fprintln(out, "1. View current config and certificate")
		fmt.Fprintln(out, "2. List profiles")
		fmt.Fprintln(out, "3. Show profile")
		fmt.Fprintln(out, "4. Import current client files as profile")
		fmt.Fprintln(out, "5. Import profile from files")
		fmt.Fprintln(out, "6. Switch active profile")
		fmt.Fprintln(out, "7. Delete profile")
		fmt.Fprintln(out, "8. Export mobile profile")
		fmt.Fprintln(out, "9. Help")
		fmt.Fprintln(out, "q. Quit")

		choice, err := promptLine(reader, out, "Select")
		if err != nil {
			return err
		}
		switch strings.ToLower(strings.TrimSpace(choice)) {
		case "1":
			printInteractiveError(out, commandCurrent(out, opts))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "2":
			printInteractiveError(out, commandList(out, opts))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "3":
			name, err := promptRequired(reader, out, "Profile name")
			if err != nil {
				return err
			}
			printInteractiveError(out, commandShow(out, opts, []string{name}))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "4":
			name, err := promptRequired(reader, out, "New profile name")
			if err != nil {
				return err
			}
			args := []string{name}
			if _, err := getProfile(opts, name); err == nil {
				yes, err := confirm(reader, out, "Profile already exists. Overwrite")
				if err != nil {
					return err
				}
				if yes {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandImportCurrent(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "5":
			name, err := promptRequired(reader, out, "New profile name")
			if err != nil {
				return err
			}
			conf, err := promptRequired(reader, out, "client.yaml path")
			if err != nil {
				return err
			}
			cert, err := promptRequired(reader, out, "server.crt path")
			if err != nil {
				return err
			}
			args := []string{name, "-conf", conf, "-cert", cert}
			if _, err := getProfile(opts, name); err == nil {
				yes, err := confirm(reader, out, "Profile already exists. Overwrite")
				if err != nil {
					return err
				}
				if yes {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandImport(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "6":
			name, err := promptRequired(reader, out, "Profile name")
			if err != nil {
				return err
			}
			printInteractiveError(out, commandUse(out, opts, []string{name}))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "7":
			name, err := promptRequired(reader, out, "Profile name")
			if err != nil {
				return err
			}
			yes, err := confirm(reader, out, "Delete this profile")
			if err != nil {
				return err
			}
			if !yes {
				fmt.Fprintln(out, "Delete cancelled.")
				if err := waitForEnter(reader, out); err != nil {
					return err
				}
				continue
			}
			args := []string{name, "-yes"}
			if p, err := getProfile(opts, name); err == nil && p.Active {
				force, err := confirm(reader, out, "Profile is active. Force delete")
				if err != nil {
					return err
				}
				if force {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandDelete(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "8":
			target, err := promptRequired(reader, out, "Output .lsylprofile path")
			if err != nil {
				return err
			}
			args := []string{"-out", target}
			if fileExists(target) {
				yes, err := confirm(reader, out, "Output already exists. Overwrite")
				if err != nil {
					return err
				}
				if yes {
					args = append(args, "-force")
				}
			}
			printInteractiveError(out, commandExportMobile(out, opts, args))
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "9", "h", "help", "?":
			printUsage(out)
			if err := waitForEnter(reader, out); err != nil {
				return err
			}
		case "q", "quit", "exit":
			fmt.Fprintln(out, "Bye.")
			return nil
		default:
			fmt.Fprintln(out, "Unknown selection.")
		}
	}
}

func promptLine(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprintf(out, "%s> ", label)
	line, err := reader.ReadString('\n')
	if err != nil && !(errors.Is(err, io.EOF) && strings.TrimSpace(line) != "") {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func promptRequired(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	for {
		value, err := promptLine(reader, out, label)
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintln(out, "Value is required.")
	}
}

func confirm(reader *bufio.Reader, out io.Writer, label string) (bool, error) {
	for {
		value, err := promptLine(reader, out, label+" [y/N]")
		if err != nil {
			return false, err
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "", "n", "no":
			return false, nil
		case "y", "yes":
			return true, nil
		default:
			fmt.Fprintln(out, "Please answer y or n.")
		}
	}
}

func waitForEnter(reader *bufio.Reader, out io.Writer) error {
	fmt.Fprint(out, "\nPress Enter to continue...")
	_, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	fmt.Fprintln(out, "")
	return nil
}

func printInteractiveError(out io.Writer, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(out, "\nERROR: %v\n", err)
	if strings.Contains(strings.ToLower(err.Error()), "access is denied") {
		fmt.Fprintln(out, "Tip: switch operations under Program Files usually need an Administrator CMD.")
	}
}
