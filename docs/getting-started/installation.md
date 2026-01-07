# Installation

## Go Install

The easiest way to install Bear is via `go install`:

```bash
go install github.com/irevolve/bear@latest
```

This installs the `bear` binary to your `$GOPATH/bin`.

## Build from Source

Clone the repository and build:

```bash
git clone https://github.com/irevolve/bear.git
cd bear
go build -o bear .
```

Move the binary to your PATH:

```bash
sudo mv bear /usr/local/bin/
```

## Verify Installation

Check that Bear is installed correctly:

```bash
bear --version
```

## Requirements

- **Git** — Bear uses Git for change detection
- **Go 1.21+** — Only needed if building from source

## Shell Completion

Bear supports shell completions for Bash, Zsh, Fish, and PowerShell.

=== "Bash"

    ```bash
    bear completion bash > /etc/bash_completion.d/bear
    ```

=== "Zsh"

    ```bash
    bear completion zsh > "${fpath[1]}/_bear"
    ```

=== "Fish"

    ```bash
    bear completion fish > ~/.config/fish/completions/bear.fish
    ```

=== "PowerShell"

    ```powershell
    bear completion powershell | Out-String | Invoke-Expression
    ```
