package main

import (
	"flag"
	"fmt"
	"io"
	"os"
)

const (
	defaultInstallDirName = `LSYL Tunnel Client`
	profileEnvName        = "LSYL_TUNNEL_PROFILE_ROOT"
	installEnvName        = "LSYL_TUNNEL_CLIENT_HOME"
)

type options struct {
	installDir  string
	profilesDir string
}

type profileInfo struct {
	Name     string
	Path     string
	ConfFile string
	CertFile string
	Valid    bool
	Active   bool
}

type pathInfo struct {
	Path      string
	Exists    bool
	Kind      string
	Effective string
}

type switchChange struct {
	Path      string
	Backup    string
	OldTarget string
	Created   bool
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("lsyl-tunnel-profile", flag.ContinueOnError)
	fs.SetOutput(out)
	installDir := fs.String("install", "", "LSYL Tunnel Client install directory")
	profilesDir := fs.String("profiles", "", "profile root directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	opts, err := resolveOptions(*installDir, *profilesDir)
	if err != nil {
		return err
	}
	remaining := fs.Args()
	if len(remaining) == 0 {
		return runInteractive(out, opts)
	}

	switch remaining[0] {
	case "current":
		return commandCurrent(out, opts)
	case "list":
		return commandList(out, opts)
	case "show":
		return commandShow(out, opts, remaining[1:])
	case "import":
		return commandImport(out, opts, remaining[1:])
	case "import-current":
		return commandImportCurrent(out, opts, remaining[1:])
	case "export-mobile":
		return commandExportMobile(out, opts, remaining[1:])
	case "use":
		return commandUse(out, opts, remaining[1:])
	case "delete":
		return commandDelete(out, opts, remaining[1:])
	case "help", "-h", "--help", "/?":
		printUsage(out)
		return nil
	default:
		return fmt.Errorf("unknown command %q", remaining[0])
	}
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "LSYL Tunnel profile manager")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] current")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] list")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] show NAME")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] import NAME -conf client.yaml -cert server.crt [-force]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] import-current NAME [-force]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] export-mobile -out mobile.lsylprofile [-profile NAME] [-force]")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] use NAME")
	fmt.Fprintln(out, "  lsyl-tunnel-profile [-install DIR] [-profiles DIR] delete NAME -yes")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Defaults:")
	fmt.Fprintln(out, "  -install  uses LSYL_TUNNEL_CLIENT_HOME, a detected client install beside this tool, or Program Files.")
	fmt.Fprintln(out, "  -profiles uses LSYL_TUNNEL_PROFILE_ROOT or ProgramData\\LSYL Tunnel Profiles.")
}
