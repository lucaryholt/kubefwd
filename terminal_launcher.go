package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// TerminalType represents the detected terminal emulator
type TerminalType int

const (
	TerminalUnknown TerminalType = iota
	TerminalITerm2
	TerminalAppleTerminal
	TerminalKitty
	TerminalAlacritty
	TerminalTmux
	TerminalVSCode
	TerminalWezTerm
)

// DetectTerminal detects which terminal emulator is being used
func DetectTerminal() TerminalType {
	// Check TERM_PROGRAM environment variable (most reliable)
	termProgram := os.Getenv("TERM_PROGRAM")
	switch termProgram {
	case "iTerm.app":
		return TerminalITerm2
	case "Apple_Terminal":
		return TerminalAppleTerminal
	case "vscode":
		return TerminalVSCode
	case "WezTerm":
		return TerminalWezTerm
	}

	// Check for tmux
	if os.Getenv("TMUX") != "" {
		return TerminalTmux
	}

	// Check for kitty
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return TerminalKitty
	}

	// Check for alacritty
	if os.Getenv("ALACRITTY_SOCKET") != "" || strings.Contains(os.Getenv("TERM"), "alacritty") {
		return TerminalAlacritty
	}

	return TerminalUnknown
}

// GetTerminalName returns a human-readable name for the terminal type
func GetTerminalName(termType TerminalType) string {
	switch termType {
	case TerminalITerm2:
		return "iTerm2"
	case TerminalAppleTerminal:
		return "Terminal.app"
	case TerminalKitty:
		return "Kitty"
	case TerminalAlacritty:
		return "Alacritty"
	case TerminalTmux:
		return "tmux"
	case TerminalVSCode:
		return "VS Code"
	case TerminalWezTerm:
		return "WezTerm"
	default:
		return "Unknown"
	}
}

// LaunchSqlTapInNewTab launches the sql-tap TUI client in a new terminal tab/window
func LaunchSqlTapInNewTab(grpcPort int) error {
	termType := DetectTerminal()
	debugLog("Detected terminal: %s", GetTerminalName(termType))

	switch termType {
	case TerminalITerm2:
		return launchInITerm2(grpcPort)
	case TerminalAppleTerminal:
		return launchInAppleTerminal(grpcPort)
	case TerminalKitty:
		return launchInKitty(grpcPort)
	case TerminalTmux:
		return launchInTmux(grpcPort)
	case TerminalWezTerm:
		return launchInWezTerm(grpcPort)
	default:
		// Fallback: try to launch in a new process (less ideal)
		return launchFallback(grpcPort)
	}
}

// launchInITerm2 launches sql-tap in a new iTerm2 tab using AppleScript
func launchInITerm2(grpcPort int) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("iTerm2 is only available on macOS")
	}

	sqlTapCmd := fmt.Sprintf("sql-tap localhost:%d", grpcPort)
	
	// AppleScript to create a new tab and run the command
	appleScript := fmt.Sprintf(`
		tell application "iTerm"
			tell current window
				create tab with default profile
				tell current session
					write text "%s"
				end tell
			end tell
		end tell
	`, sqlTapCmd)

	cmd := exec.Command("osascript", "-e", appleScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to launch iTerm2 tab: %w", err)
	}

	debugLog("Launched sql-tap in new iTerm2 tab: %s", sqlTapCmd)
	return nil
}

// launchInAppleTerminal launches sql-tap in a new Terminal.app tab using AppleScript
func launchInAppleTerminal(grpcPort int) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("Terminal.app is only available on macOS")
	}

	sqlTapCmd := fmt.Sprintf("sql-tap localhost:%d", grpcPort)
	
	// AppleScript to create a new tab and run the command
	appleScript := fmt.Sprintf(`
		tell application "Terminal"
			activate
			tell application "System Events"
				keystroke "t" using command down
			end tell
			delay 0.5
			do script "%s" in front window
		end tell
	`, sqlTapCmd)

	cmd := exec.Command("osascript", "-e", appleScript)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to launch Terminal.app tab: %w", err)
	}

	debugLog("Launched sql-tap in new Terminal.app tab: %s", sqlTapCmd)
	return nil
}

// launchInKitty launches sql-tap in a new kitty tab
func launchInKitty(grpcPort int) error {
	sqlTapCmd := fmt.Sprintf("sql-tap localhost:%d", grpcPort)
	
	// Use kitty remote control to launch in new tab
	cmd := exec.Command("kitty", "@", "launch", "--type=tab", "--tab-title=sql-tap", "sql-tap", fmt.Sprintf("localhost:%d", grpcPort))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch kitty tab: %w", err)
	}

	debugLog("Launched sql-tap in new kitty tab: %s", sqlTapCmd)
	return nil
}

// launchInTmux launches sql-tap in a new tmux window
func launchInTmux(grpcPort int) error {
	sqlTapCmd := fmt.Sprintf("sql-tap localhost:%d", grpcPort)
	
	// Create a new tmux window with the command
	cmd := exec.Command("tmux", "new-window", "-n", "sql-tap", sqlTapCmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to launch tmux window: %w", err)
	}

	debugLog("Launched sql-tap in new tmux window: %s", sqlTapCmd)
	return nil
}

// launchInWezTerm launches sql-tap in a new WezTerm tab
func launchInWezTerm(grpcPort int) error {
	// WezTerm CLI to spawn a new tab
	cmd := exec.Command("wezterm", "cli", "spawn", "--new-window", "sql-tap", fmt.Sprintf("localhost:%d", grpcPort))
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch WezTerm tab: %w", err)
	}

	debugLog("Launched sql-tap in new WezTerm tab")
	return nil
}

// launchFallback attempts to launch sql-tap in a fallback manner
func launchFallback(grpcPort int) error {
	// This is less ideal, but we can try to launch in a detached process
	// The user should see the command to run manually
	return fmt.Errorf("terminal type not detected - please run manually: sql-tap localhost:%d", grpcPort)
}

// GetSqlTapLaunchCommand returns the command string to manually launch sql-tap
func GetSqlTapLaunchCommand(grpcPort int) string {
	return fmt.Sprintf("sql-tap localhost:%d", grpcPort)
}
