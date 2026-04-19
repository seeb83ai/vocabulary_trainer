package migrate

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func init() {
	register(migration{
		// v20: create users table and seed the two initial users.
		// admin@... (id=1) is the template user whose vocabulary serves as
		// the importable baseline for new users. The personal user (id=2) is
		// the original single user who owns all pre-migration data.
		//
		// Credentials are read from env vars (ADMIN_EMAIL, ADMIN_PASSWORD,
		// USER_EMAIL, USER_PASSWORD) if set, otherwise prompted interactively.
		// When running in Docker, pass -it or set the env vars.
		version: 20,
		sql: `
CREATE TABLE IF NOT EXISTS users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT    NOT NULL UNIQUE,
  password_hash TEXT    NOT NULL,
  created_at    DATETIME DEFAULT CURRENT_TIMESTAMP
);
`,
		fn: func(db *sql.DB) error {
			fmt.Fprintln(os.Stderr, "\n┌─────────────────────────────────────────────────────┐")
			fmt.Fprintln(os.Stderr, "│  First-run setup: creating user accounts            │")
			fmt.Fprintln(os.Stderr, "│                                                     │")
			fmt.Fprintln(os.Stderr, "│  User 1 (admin): owns the template word library.    │")
			fmt.Fprintln(os.Stderr, "│  User 2 (you):   owns your vocabulary & progress.   │")
			fmt.Fprintln(os.Stderr, "│                                                     │")
			fmt.Fprintln(os.Stderr, "│  Tip: set ADMIN_EMAIL / ADMIN_PASSWORD /             │")
			fmt.Fprintln(os.Stderr, "│       USER_EMAIL  / USER_PASSWORD to skip prompts.  │")
			fmt.Fprintln(os.Stderr, "└─────────────────────────────────────────────────────┘")

			adminEmail, adminPass, err := promptCredentials("Admin (template user)", "ADMIN_EMAIL", "ADMIN_PASSWORD")
			if err != nil {
				return fmt.Errorf("admin credentials: %w", err)
			}
			userEmail, userPass, err := promptCredentials("Your personal account", "USER_EMAIL", "USER_PASSWORD")
			if err != nil {
				return fmt.Errorf("user credentials: %w", err)
			}

			adminHash, err := bcrypt.GenerateFromPassword([]byte(adminPass), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash admin password: %w", err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)`,
				adminEmail, string(adminHash),
			); err != nil {
				return fmt.Errorf("seed admin user: %w", err)
			}

			userHash, err := bcrypt.GenerateFromPassword([]byte(userPass), bcrypt.DefaultCost)
			if err != nil {
				return fmt.Errorf("hash user password: %w", err)
			}
			if _, err := db.Exec(
				`INSERT OR IGNORE INTO users (email, password_hash) VALUES (?, ?)`,
				userEmail, string(userHash),
			); err != nil {
				return fmt.Errorf("seed personal user: %w", err)
			}

			fmt.Fprintf(os.Stderr, "\nUsers created: %s (admin) and %s\n\n", adminEmail, userEmail)
			return nil
		},
	})
}

// promptCredentials reads email and password for the given account label.
// It checks emailEnv / passEnv environment variables first; if both are set
// the user is not prompted. Password input is hidden when stdin is a terminal.
func promptCredentials(label, emailEnv, passEnv string) (email, password string, err error) {
	email = os.Getenv(emailEnv)
	password = os.Getenv(passEnv)
	if email != "" && password != "" {
		fmt.Fprintf(os.Stderr, "%s: using credentials from environment (%s)\n", label, email)
		return email, password, nil
	}

	fmt.Fprintf(os.Stderr, "\n--- %s ---\n", label)
	reader := bufio.NewReader(os.Stdin)

	fmt.Fprintf(os.Stderr, "  Email: ")
	email, err = reader.ReadString('\n')
	if err != nil {
		return "", "", fmt.Errorf("reading email: %w", err)
	}
	email = strings.TrimSpace(email)
	if email == "" {
		return "", "", fmt.Errorf("email cannot be empty")
	}

	fmt.Fprintf(os.Stderr, "  Password: ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		passBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr) // newline after hidden input
		if err != nil {
			return "", "", fmt.Errorf("reading password: %w", err)
		}
		password = string(passBytes)
	} else {
		password, err = reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("reading password: %w", err)
		}
		password = strings.TrimSpace(password)
	}
	if password == "" {
		return "", "", fmt.Errorf("password cannot be empty")
	}

	return email, password, nil
}
