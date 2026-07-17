package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	device "github.com/mtgo-labs/device-manager"
	"github.com/mtgo-labs/mtgo/telegram"
	"github.com/mtgo-labs/session-generator/relogin"
	tgconv "github.com/mtgo-labs/session-converter"
)

func cmdRelogin(args []string) {
	fs := flag.NewFlagSet("relogin", flag.ExitOnError)
	apiID := fs.Int64("api-id", 0, "API ID from my.telegram.org (required)")
	apiHash := fs.String("api-hash", "", "API Hash from my.telegram.org (required)")
	to := fs.String("t", "telethon", "output format")
	devType := fs.String("device", "android", "device type: android, android_x, ios, macos, windows, linux, desktop, web_z, web_k, webogram")
	deviceID := fs.String("device-id", "", "unique ID for deterministic device generation (random if empty)")
	fs.Parse(reorderArgs(args))

	rest := fs.Args()
	if *apiID == 0 || *apiHash == "" || len(rest) < 1 {
		fmt.Fprintln(os.Stderr, "usage: tgconv relogin <session-string> --api-id <id> --api-hash <hash> [-t <format>] [--device <type>] [--device-id <id>]")
		os.Exit(1)
	}

	sessionStr := rest[0]

	// Load from file if it looks like a path.
	if strings.HasSuffix(sessionStr, ".session") {
		s, fmtName, err := ReadSQLite(sessionStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: read sqlite: %v\n", err)
			os.Exit(1)
		}
		sessionStr, err = tgconv.Encode(s, tgconv.FormatPyrogram)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: encode sqlite session: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "relogin: loaded %s session from %s\n", fmtName, rest[0])
	}

	// Validate format.
	format := tgconv.Format(*to)
	if !validFormat(format) {
		fmt.Fprintf(os.Stderr, "error: unsupported format %q\n", *to)
		os.Exit(1)
	}

	// Parse device type.
	dt := parseDeviceType(*devType)

	opts := relogin.Options{
		APIID:      *apiID,
		APIHash:    *apiHash,
		Session:    sessionStr,
		Format:     format,
		CodeFunc:   telegram.TerminalCodeFunc(),
		PassFunc:   telegram.TerminalPasswordFunc(),
		DeviceType: dt,
		DeviceID:   *deviceID,
		Output:     os.Stderr,
	}

	result, err := relogin.Relogin(context.Background(), opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "relogin: session generated successfully")
	fmt.Println(result.Session)
}

func parseDeviceType(s string) device.Device {
	switch strings.ToLower(s) {
	case "android", "":
		return device.Android
	case "android_x", "androidx":
		return device.AndroidX
	case "ios":
		return device.IOS
	case "macos":
		return device.MacOS
	case "windows":
		return device.Windows
	case "linux":
		return device.Linux
	case "desktop":
		return device.Desktop
	case "web_z", "webz":
		return device.WebZ
	case "web_k", "webk":
		return device.WebK
	case "webogram":
		return device.Webogram
	default:
		return device.Android
	}
}
