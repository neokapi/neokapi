package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "keygen":
		if err := runKeygen(); err != nil {
			fail(err)
		}
	case "gen":
		if err := runGen(os.Args[2:]); err != nil {
			fail(err)
		}
	case "-h", "--help", "help":
		usage()
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `mkappcast — generate a signed AppCast feed for the Wails v3 native updater.

Usage:
  mkappcast keygen
      Generate an ed25519 key pair. Prints the base64 PUBLIC key (commit it to
      each desktop app's build/update-ed25519.pub) and the base64 PRIVATE key
      (store as the UPDATE_ED25519_PRIVATE_KEY CI secret — never commit it).

  mkappcast gen --title <name> --version <x.y.z> [--channel beta] \
      --url-prefix <https://.../download/vX.Y.Z> --out <appcast.xml> <artifact.zip>
      Sign the artifact's SHA-256 digest and write an appcast feed with one item.
      The ed25519 private key is read from $UPDATE_ED25519_PRIVATE_KEY (base64).
`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "mkappcast:", err)
	os.Exit(1)
}

func runKeygen() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	fmt.Printf("public  %s\n", base64.StdEncoding.EncodeToString(pub))
	fmt.Printf("private %s\n", base64.StdEncoding.EncodeToString(priv))
	return nil
}

func runGen(args []string) error {
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	title := fs.String("title", "", "app title (e.g. Kapi)")
	ver := fs.String("version", "", "short version, e.g. 1.2.0")
	channel := fs.String("channel", "", "channel tag (e.g. beta); empty for default")
	urlPrefix := fs.String("url-prefix", "", "download URL prefix for the enclosure")
	out := fs.String("out", "", "output appcast path")
	pubDate := fs.String("pubdate", "", "RFC1123Z pub date (default: now)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *title == "" || *ver == "" || *out == "" {
		return fmt.Errorf("--title, --version and --out are required")
	}
	rest := fs.Args()
	if len(rest) != 1 {
		return fmt.Errorf("exactly one artifact path is required, got %d", len(rest))
	}
	artifact := rest[0]

	priv, err := privateKeyFromEnv()
	if err != nil {
		return err
	}

	date := *pubDate
	if date == "" {
		date = nowRFC1123Z(time.Now())
	}
	item, err := newItem(priv, *ver, *channel, downloadURL(*urlPrefix, artifact), artifact, date)
	if err != nil {
		return err
	}
	xmlDoc := renderAppcast(*title, []Item{item})
	if err := os.WriteFile(*out, []byte(xmlDoc), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", *out, err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%s %s, channel=%q)\n", *out, *title, *ver, *channel)
	return nil
}

// privateKeyFromEnv reads the base64 ed25519 private key from
// UPDATE_ED25519_PRIVATE_KEY and validates its length.
func privateKeyFromEnv() (ed25519.PrivateKey, error) {
	raw := strings.TrimSpace(os.Getenv("UPDATE_ED25519_PRIVATE_KEY"))
	if raw == "" {
		return nil, fmt.Errorf("UPDATE_ED25519_PRIVATE_KEY is not set")
	}
	key, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("decode UPDATE_ED25519_PRIVATE_KEY: %w", err)
	}
	if len(key) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("private key is %d bytes, want %d", len(key), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(key), nil
}
