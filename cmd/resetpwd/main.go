// Command resetpwd safely resets the single admin password for a Private Drop
// instance. It never logs or echoes the password.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"

	"private-drop/internal/audit"
	"private-drop/internal/auth"
	"private-drop/internal/config"
	"private-drop/internal/db"
)

func main() {
	dataDir := flag.String("data-dir", "./data", "path to the Private Drop data directory")
	username := flag.String("username", "", "admin username (default: the only account)")
	passwordFlag := flag.String("new-password", "", "new password (discouraged; prefer stdin)")
	flag.Parse()

	cfg := config.Config{DataDir: *dataDir}
	if err := cfg.EnsureDirs(); err != nil {
		fail(err)
	}
	password := *passwordFlag
	if password == "" {
		fmt.Fprint(os.Stderr, "New password: ")
		if term.IsTerminal(int(os.Stdin.Fd())) {
			b, err := term.ReadPassword(int(os.Stdin.Fd()))
			fmt.Fprintln(os.Stderr)
			if err != nil {
				fail(err)
			}
			password = string(b)
		} else {
			scanner := bufio.NewScanner(os.Stdin)
			if !scanner.Scan() {
				fail(fmt.Errorf("no password provided on stdin"))
			}
			password = strings.TrimRight(scanner.Text(), "\r\n")
		}
	}
	if len(password) < 8 {
		fail(fmt.Errorf("password must be at least 8 characters"))
	}

	database, err := db.Open(cfg.DBPath())
	if err != nil {
		fail(err)
	}
	defer database.Close()
	if err := database.Migrate(); err != nil {
		fail(err)
	}

	ctx := context.Background()
	var id, storedUser string
	if *username != "" {
		err = database.QueryRowContext(ctx, `SELECT id,username FROM users WHERE username=?`, *username).Scan(&id, &storedUser)
	} else {
		err = database.QueryRowContext(ctx, `SELECT id,username FROM users ORDER BY rowid LIMIT 1`).Scan(&id, &storedUser)
	}
	if err != nil {
		fail(fmt.Errorf("admin account not found: %w", err))
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fail(err)
	}
	if _, err := database.ExecContext(ctx, `UPDATE users SET password_hash=? WHERE id=?`, hash, id); err != nil {
		fail(err)
	}
	// Record the security-relevant action. The password (and its hash) are
	// deliberately never written to the audit log.
	audit.New(database).Log(ctx, "security.password_reset", "通过 resetpwd CLI 重置密码: "+storedUser, "", "")
	// Never print the password itself.
	fmt.Printf("Password reset for user %q. All sessions remain valid; revoke devices in the admin UI if needed.\n", storedUser)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "resetpwd:", err)
	os.Exit(1)
}
