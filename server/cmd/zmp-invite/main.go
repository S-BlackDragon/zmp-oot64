// Command zmp-invite makes and reads ZMP invite codes (the protocol 5 design notes section 7), for tests and for
// anyone who wants a code by hand:
//
//	zmp-invite encode -addr 203.0.113.7 -addr 192.168.1.20 -room bosque -password saria
//	zmp-invite decode ZMP-0410-416B-...
//
// encode prints the text, the link and the binary in hex; decode prints the fields as JSON. Exit code 1 on a bad
// code, 2 on bad arguments.
package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/S-BlackDragon/zmp-oot64/server/internal/invite"
)

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(errOut, "usage: zmp-invite encode -addr A [-addr B ...] [-port P] -room R [-password P | -hint H] [-json]")
		fmt.Fprintln(errOut, "       zmp-invite decode <code or zmp:// link>")
		return 2
	}
	switch args[0] {
	case "encode":
		fs := flag.NewFlagSet("encode", flag.ContinueOnError)
		fs.SetOutput(errOut)
		var addrs list
		fs.Var(&addrs, "addr", "address in the order clients try them: IP, host name or ws(s):// URL (1 to 4)")
		port := fs.Int("port", invite.DefaultPort, "port of the IP and host name addresses")
		room := fs.String("room", "", "room")
		password := fs.String("password", "", "room password to put in the code")
		hint := fs.String("hint", "", "only a hint of the password (the friend types it)")
		asJSON := fs.Bool("json", false, "print JSON {text, link, hex}")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		inv := invite.Invite{Addresses: addrs, Port: *port, Room: *room, Password: *password, Hint: *hint}
		b, err := invite.Encode(inv)
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		text, _ := invite.Format(inv)
		link, _ := invite.Link(inv)
		if *asJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]string{"text": text, "link": link, "hex": hex.EncodeToString(b)})
			return 0
		}
		fmt.Fprintln(out, text)
		fmt.Fprintln(out, link)
		fmt.Fprintln(out, hex.EncodeToString(b))
		return 0
	case "decode":
		if len(args) < 2 {
			fmt.Fprintln(errOut, "decode: missing code")
			return 2
		}
		inv, err := invite.Parse(strings.Join(args[1:], " "))
		if err != nil {
			fmt.Fprintln(errOut, err)
			return 1
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"addresses": inv.Addresses, "port": inv.Port, "room": inv.Room,
			"password": inv.Password, "hint": inv.Hint})
		return 0
	}
	fmt.Fprintf(errOut, "unknown command %q (encode or decode)\n", args[0])
	return 2
}
