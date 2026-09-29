package picker

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

var ErrCancelled = errors.New("selection cancelled")

type pathLookup func(string) (string, error)
type commandRunner func(context.Context, string, ...string) (string, error)

type dialogCommand struct {
	name string
	args []string
}

func Files(ctx context.Context) ([]string, error) {
	paths, err := pickFiles(ctx, runtime.GOOS, exec.LookPath, runCommand)
	if errors.Is(err, ErrCancelled) {
		return nil, nil
	}
	return paths, err
}

func Folder(ctx context.Context) (string, error) {
	path, err := pickFolder(ctx, runtime.GOOS, exec.LookPath, runCommand)
	if errors.Is(err, ErrCancelled) {
		return "", nil
	}
	return path, err
}

func pickFiles(ctx context.Context, platform string, lookup pathLookup, run commandRunner) ([]string, error) {
	command, err := filesCommand(platform, lookup)
	if err != nil {
		return nil, err
	}
	output, err := run(ctx, command.name, command.args...)
	if err != nil {
		if dialogCancelled(platform, output, err) {
			return nil, ErrCancelled
		}
		return nil, fmt.Errorf("open file picker: %w", err)
	}
	paths := outputPaths(output)
	if len(paths) == 0 {
		return nil, ErrCancelled
	}
	return paths, nil
}

func pickFolder(ctx context.Context, platform string, lookup pathLookup, run commandRunner) (string, error) {
	command, err := folderCommand(platform, lookup)
	if err != nil {
		return "", err
	}
	output, err := run(ctx, command.name, command.args...)
	if err != nil {
		if dialogCancelled(platform, output, err) {
			return "", ErrCancelled
		}
		return "", fmt.Errorf("open folder picker: %w", err)
	}
	paths := outputPaths(output)
	if len(paths) == 0 {
		return "", ErrCancelled
	}
	return paths[0], nil
}

func filesCommand(platform string, lookup pathLookup) (dialogCommand, error) {
	switch platform {
	case "darwin":
		script := `set selectedFiles to choose file with prompt "Choose documents to import" with multiple selections allowed
set output to ""
repeat with selectedFile in selectedFiles
  set output to output & POSIX path of selectedFile & linefeed
end repeat
return output`
		return dialogCommand{name: "osascript", args: []string{"-e", script}}, nil
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms; $dialog = New-Object System.Windows.Forms.OpenFileDialog; $dialog.Multiselect = $true; $dialog.CheckFileExists = $true; $dialog.Title = 'Choose documents to import'; if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $dialog.FileNames }`
		return dialogCommand{name: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", script}}, nil
	case "linux":
		return linuxCommand(lookup,
			dialogCommand{name: "zenity", args: []string{"--file-selection", "--multiple", "--separator=\n", "--title=Choose documents to import"}},
			dialogCommand{name: "kdialog", args: []string{"--getopenfilename", ".", "", "--multiple", "--separate-output"}},
		)
	default:
		return dialogCommand{}, fmt.Errorf("graphical file picker is not supported on %s", platform)
	}
}

func folderCommand(platform string, lookup pathLookup) (dialogCommand, error) {
	switch platform {
	case "darwin":
		return dialogCommand{name: "osascript", args: []string{"-e", `return POSIX path of (choose folder with prompt "Choose a folder to import")`}}, nil
	case "windows":
		script := `Add-Type -AssemblyName System.Windows.Forms; $dialog = New-Object System.Windows.Forms.FolderBrowserDialog; $dialog.Description = 'Choose a folder to import'; if ($dialog.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK) { $dialog.SelectedPath }`
		return dialogCommand{name: "powershell.exe", args: []string{"-NoProfile", "-NonInteractive", "-Command", script}}, nil
	case "linux":
		return linuxCommand(lookup,
			dialogCommand{name: "zenity", args: []string{"--file-selection", "--directory", "--title=Choose a folder to import"}},
			dialogCommand{name: "kdialog", args: []string{"--getexistingdirectory", "."}},
		)
	default:
		return dialogCommand{}, fmt.Errorf("graphical folder picker is not supported on %s", platform)
	}
}

func linuxCommand(lookup pathLookup, zenity, kdialog dialogCommand) (dialogCommand, error) {
	if path, err := lookup(zenity.name); err == nil {
		zenity.name = path
		return zenity, nil
	}
	if path, err := lookup(kdialog.name); err == nil {
		kdialog.name = path
		return kdialog, nil
	}
	return dialogCommand{}, errors.New("graphical picker requires Zenity or KDialog on Linux")
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(output), err
}

func outputPaths(output string) []string {
	output = strings.ReplaceAll(output, "\r\n", "\n")
	lines := strings.Split(strings.TrimSpace(output), "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		if path := strings.TrimSpace(line); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func dialogCancelled(platform, output string, err error) bool {
	if platform == "darwin" {
		return strings.Contains(output, "(-128)") || strings.Contains(strings.ToLower(output), "user canceled")
	}
	var exitError *exec.ExitError
	return platform == "linux" && errors.As(err, &exitError) && exitError.ExitCode() == 1
}
