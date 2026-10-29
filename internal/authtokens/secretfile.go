package authtokens

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dash0hq/dash0-cli/internal/agentmode"
	"github.com/dash0hq/dash0-cli/internal/git"
)

// errNoSecretDestination is returned before any API call when create has
// nowhere safe to deliver the secret.
var errNoSecretDestination = errors.New("the secret of a new auth token is shown only once, and there is no interactive terminal to show it on\n" +
	"Hint: pass --secret-file <path> to write it to a new file that only you can read")

// secretOnTerminal reports whether create may print the secret on stdout.
// That needs a person watching: an interactive terminal, agent mode off, and
// the human-readable output format. Everything else needs --secret-file.
func secretOnTerminal(stdoutIsTerminal bool, format createFormat) bool {
	return stdoutIsTerminal && !agentmode.Enabled && format == createFormatTable
}

// openSecretFile creates path for the secret before the token exists, so a
// path problem fails without leaving a token whose secret is lost.
// The file must be new (O_EXCL, which also refuses a symlink), is readable
// only by the user, and must not be a file git would commit.
func openSecretFile(ctx context.Context, path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("invalid --secret-file path %q: %w", path, err)
	}
	if err := requireNotCommittable(ctx, abs); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("--secret-file %q already exists; pass a path to a new file", path)
		}
		return nil, fmt.Errorf("failed to create --secret-file %q: %w", path, err)
	}
	// The umask can only remove permission bits, but set the mode explicitly so
	// the file is 0600 whatever the open call left.
	if err := file.Chmod(0o600); err != nil {
		discardSecretFile(file)
		return nil, fmt.Errorf("failed to restrict --secret-file %q to mode 0600: %w", path, err)
	}
	return file, nil
}

// requireNotCommittable refuses a path inside a git work tree unless git
// ignores it, so the secret does not end up in a commit.
func requireNotCommittable(ctx context.Context, path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid --secret-file path %q: %w", path, err)
	}
	dir := filepath.Dir(abs)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return fmt.Errorf("--secret-file directory %q does not exist", dir)
	}
	repo := git.Repo{Dir: dir}
	_, err = repo.Root(ctx)
	switch {
	case err == nil:
	case git.IsNotAGitRepository(err):
		return nil
	case errors.Is(err, exec.ErrNotFound):
		// Without git the ignore rules cannot be read, so any work tree is refused.
		if insideWorkTree(dir) {
			return fmt.Errorf("--secret-file %q is inside a git work tree, and git is not installed to check whether the path is ignored\n"+
				"Hint: pass a path outside the repository", abs)
		}
		return nil
	default:
		return fmt.Errorf("failed to check whether --secret-file %q is inside a git repository: %w", abs, err)
	}
	ignored, err := repo.IsIgnored(ctx, abs)
	if err != nil {
		return fmt.Errorf("failed to check whether git ignores --secret-file %q: %w", abs, err)
	}
	if !ignored {
		return fmt.Errorf("--secret-file %q is inside a git repository and git does not ignore it, so the secret could be committed\n"+
			"Hint: pass a path outside the repository, or add the path to .gitignore", abs)
	}
	return nil
}

func insideWorkTree(dir string) bool {
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// writeSecretFile writes the secret and a trailing newline, then closes the file.
func writeSecretFile(file *os.File, value secret) error {
	if _, err := file.WriteString(value.reveal() + "\n"); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// discardSecretFile removes a secret file that never received a secret.
func discardSecretFile(file *os.File) {
	_ = file.Close()
	_ = os.Remove(file.Name())
}
