// Command castrecord records the raw Cast protocol exchange with one device, READ-ONLY
// (it can only ask for status; see internal/castrecord), and writes it as JSON lines.
//
//	castrecord -host 192.168.1.20 -port 8009 -for 5m -out raw.jsonl
//
// The raw file identifies the household (addresses, names); run it through -scrub
// before committing anything as a test fixture.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/vishen/go-chromecast/application"
	"github.com/vishen/go-chromecast/cast"

	"azan-agent/internal/castrecord"
)

func main() {
	host := flag.String("host", "", "device address")
	port := flag.Int("port", 8009, "device port")
	dur := flag.Duration("for", 30*time.Second, "how long to record")
	every := flag.Duration("every", time.Second, "status poll interval")
	out := flag.String("out", "", "output file (JSON lines)")
	scrub := flag.String("scrub", "", "comma list of old=new name replacements; enables scrubbing of IPs, ids and names")
	convert := flag.String("convert", "", "instead of recording: scrub this existing raw JSONL file into -out (uses -scrub)")
	flag.Parse()
	if *convert != "" {
		convertFile(*convert, *out, *scrub)
		return
	}
	if *host == "" || *out == "" {
		flag.Usage()
		os.Exit(2)
	}

	rec := castrecord.New(cast.NewConnection())
	app := application.NewApplication(application.WithConnection(rec), application.WithCacheDisabled(true))
	if err := app.Start(*host, *port); err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer app.Close(false)

	end := time.Now().Add(*dur)
	for time.Now().Before(end) {
		if err := app.Update(); err != nil {
			log.Printf("update: %v", err)
		}
		time.Sleep(*every)
	}

	names := parseNames(*scrub)
	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	frames := rec.Frames()
	for _, fr := range frames {
		b, _ := json.Marshal(fr)
		line := string(b)
		if *scrub != "" {
			line = castrecord.Scrub(line, names)
		}
		fmt.Fprintln(f, line)
	}
	log.Printf("recorded %d frames to %s", len(frames), *out)
}

func parseNames(spec string) map[string]string {
	names := map[string]string{}
	for _, kv := range strings.Split(spec, ",") {
		if k, v, ok := strings.Cut(kv, "="); ok {
			names[k] = v
		}
	}
	return names
}

// convertFile scrubs an existing raw recording line by line.
func convertFile(in, out, scrub string) {
	data, err := os.ReadFile(in)
	if err != nil {
		log.Fatal(err)
	}
	names := parseNames(scrub)
	var sb strings.Builder
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		sb.WriteString(castrecord.Scrub(line, names))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(out, []byte(sb.String()), 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("scrubbed %s -> %s", in, out)
}
