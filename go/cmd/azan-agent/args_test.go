package main

import "testing"

func TestParseArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		goos    string
		want    string
		wantErr bool
	}{
		{name: "no arguments runs the agent", goos: "linux", want: cmdRun},
		{name: "--version", args: []string{"--version"}, goos: "linux", want: cmdVersion},
		{name: "-version", args: []string{"-version"}, goos: "linux", want: cmdVersion},
		{name: "-v", args: []string{"-v"}, goos: "linux", want: cmdVersion},
		{name: "version", args: []string{"version"}, goos: "linux", want: cmdVersion},
		{name: "install", args: []string{"install"}, goos: "linux", want: cmdInstall},
		{name: "uninstall", args: []string{"uninstall"}, goos: "linux", want: cmdUninstall},
		{name: "start", args: []string{"start"}, goos: "linux", want: cmdStart},
		{name: "stop", args: []string{"stop"}, goos: "linux", want: cmdStop},

		{name: "typo in a flag", args: []string{"--versoin"}, goos: "linux", wantErr: true},
		{name: "unknown word", args: []string{"restart"}, goos: "linux", wantErr: true},
		{name: "commands are case sensitive", args: []string{"START"}, goos: "linux", wantErr: true},
		{name: "empty argument", args: []string{""}, goos: "linux", wantErr: true},
		{name: "extra argument after a flag", args: []string{"--version", "x"}, goos: "linux", wantErr: true},
		{name: "extra argument after a command", args: []string{"start", "x"}, goos: "linux", wantErr: true},
		{name: "two commands", args: []string{"stop", "start"}, goos: "linux", wantErr: true},

		// Finder can add a process serial number when it opens the app.
		{name: "macOS Finder argument", args: []string{"-psn_0_12345"}, goos: "darwin", want: cmdRun},
		{name: "Finder argument is macOS only", args: []string{"-psn_0_12345"}, goos: "linux", wantErr: true},
		{name: "Finder argument is not a command prefix", args: []string{"-psn_0_12345", "start"}, goos: "darwin", wantErr: true},
		{name: "unknown argument on macOS", args: []string{"--versoin"}, goos: "darwin", wantErr: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseArgs(c.args, c.goos)
			if c.wantErr {
				if err == nil {
					t.Fatalf("parseArgs(%q) = %q, want an error", c.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseArgs(%q): %v", c.args, err)
			}
			if got != c.want {
				t.Errorf("parseArgs(%q) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}
