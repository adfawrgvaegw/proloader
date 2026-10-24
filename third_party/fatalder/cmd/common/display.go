package common

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/gookit/color"
	"github.com/pterm/pterm"

	"github.com/Yeah114/Fatalder/constants"
	"github.com/Yeah114/Fatalder/utils"
	l10n "github.com/Yeah114/Fatalder/utils/localization"
)

// Terminal color constants for consistent CLI output styling.
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

func init() {
	enableANSIColors()
}

// ShowProgramInfo prints the ASCII banner, metadata and environment info.
func ShowProgramInfo() {
	pterm.Println(pterm.LightYellow("" +
		"╔═══════════════════════════════════════════════════════════════════╗\n" +
		"║ ███████╗ █████╗ ████████╗ █████╗ ██╗     ██████╗ ███████╗██████╗  ║\n" +
		"║ ██╔════╝██╔══██╗╚══██╔══╝██╔══██╗██║     ██╔══██╗██╔════╝██╔══██╗ ║\n" +
		"║ █████╗  ███████║   ██║   ███████║██║     ██║  ██║█████╗  ██████╔╝ ║\n" +
		"║ ██╔══╝  ██╔══██║   ██║   ██╔══██║██║     ██║  ██║██╔══╝  ██╔══██╗ ║\n" +
		"║ ██║     ██║  ██║   ██║   ██║  ██║███████╗██████╔╝███████╗██║  ██║ ║\n" +
		"║ ╚═╝     ╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝╚══════╝╚═════╝ ╚══════╝╚═╝  ╚═╝ ║\n" +
		"╚═══════════════════════════════════════════════════════════════════╝",
	))

	line1 := pterm.Sprintf("%s - v%s", constants.Name, constants.Version)
	line2 := "https://github.com/Yeah114/Fatalder-Release/releases/latest"
	spaceCount := max((len(line2)-len(line1))/2, 0)
	content := pterm.LightCyan(fmt.Sprintf("%*s", spaceCount, "") + line1 + "\n" + line2)
	pterm.DefaultBox.Println(pterm.LightCyan(content))
	fmt.Printf("%s: %s\n", l10n.T(l10n.KeyVersionAuthors), joinNames(constants.Authors))
	PrintSpecialThanks(constants.SpecialThanks)
	if constants.CommitHash != "unknown" && constants.CommitHash != "" {
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyVersionCommit), constants.CommitHash)
	}
	if constants.CommitNum > 0 {
		fmt.Printf("%s: %d\n", l10n.T(l10n.KeyVersionCommitNum), constants.CommitNum)
	}
	if constants.BuildDate != "unknown" && constants.BuildDate != "" {
		fmt.Printf("%s: %s\n", l10n.T(l10n.KeyVersionBuildDate), constants.BuildDate)
	}
	fmt.Printf("%s: %s\n", l10n.T(l10n.KeyVersionStoragePath), utils.GetStoragePath())
	fmt.Printf("%s: %s\n", l10n.T(l10n.KeyVersionLanguage), l10n.GetLanguageDisplayName(utils.GetLanguage()))
	fmt.Printf("Run on %s %s\n", runtime.GOOS, runtime.GOARCH)
}

// PrintSpecialThanks renders all contributors with a colorful gradient.
func PrintSpecialThanks(names []string) {
	if len(names) == 0 {
		return
	}
	shuffled := shuffle(names)
	n := len(shuffled)

	// Bright rainbow palette for vivid gradients.
	brightRainbow := []struct{ r, g, b int }{
		{255, 80, 80},
		{255, 180, 80},
		{255, 255, 100},
		{100, 255, 100},
		{80, 220, 255},
		{140, 140, 255},
		{220, 140, 255},
	}
	rbLen := len(brightRainbow)

	startIdx := rand.Intn(rbLen)
	endIdx := rand.Intn(rbLen)
	if startIdx == endIdx {
		endIdx = (startIdx + 1) % rbLen
	}

	totalSeg := abs(endIdx - startIdx)
	if totalSeg == 0 {
		totalSeg = rbLen - 1
	}
	segStep := n / totalSeg
	remainder := n % totalSeg

	titleColor := brightRainbow[rand.Intn(rbLen)]
	print(color.RenderCode(fmt.Sprintf("38;2;%d;%d;%d", titleColor.r, titleColor.g, titleColor.b), "SpecialThanks: "))

	for i, name := range shuffled {
		curSeg := i / segStep
		if curSeg >= totalSeg {
			curSeg = totalSeg
		}
		posInSeg := i % segStep
		curSegLen := segStep
		if curSeg == totalSeg-1 {
			curSegLen += remainder
		}

		var startCol, endCol struct{ r, g, b int }
		if startIdx < endIdx {
			startCol = brightRainbow[startIdx+curSeg]
			endCol = brightRainbow[min(startIdx+curSeg+1, endIdx)]
		} else {
			startCol = brightRainbow[startIdx-curSeg]
			endCol = brightRainbow[max(startIdx-curSeg-1, endIdx)]
		}

		curR := startCol.r + (endCol.r-startCol.r)*posInSeg/curSegLen
		curG := startCol.g + (endCol.g-startCol.g)*posInSeg/curSegLen
		curB := startCol.b + (endCol.b-startCol.b)*posInSeg/curSegLen

		print(color.RenderCode(fmt.Sprintf("38;2;%d;%d;%d", curR, curG, curB), name))
		if i != n-1 {
			print(color.RenderCode("38;2;200;200;200", ", "))
		}
	}
	println()
}

// CheckAndShowNewCommits prints new commits when the stored commit count changes.
func CheckAndShowNewCommits() {
	tempStoragePath := os.Getenv("HOME")
	if tempStoragePath == "" {
		return
	}
	tempStoragePath = filepath.Join(tempStoragePath, ".local", "share", "Fatalder")

	commitNumFile := filepath.Join(tempStoragePath, "commit_num")

	lastCommitNum := 0
	if data, err := os.ReadFile(commitNumFile); err == nil {
		if num, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
			lastCommitNum = num
		}
	}

	if constants.CommitNum > lastCommitNum {
		showNewCommits(lastCommitNum)
		updateCommitNumFile(commitNumFile, constants.CommitNum)
	}
}

func showNewCommits(lastCommitNum int) {
	newCommits := constants.GetCommitsSince(lastCommitNum)
	if len(newCommits) == 0 {
		return
	}

	for i, commit := range newCommits {
		fmt.Printf("%s%scommit %s%s\n", ColorBold, ColorYellow, commit.Hash, ColorReset)
		fmt.Printf("%sAuthor:%s %s%s%s\n", ColorBold, ColorReset, ColorCyan, strings.ReplaceAll(commit.Author, "猫七街", "耶"), ColorReset)
		fmt.Printf("%sDate:%s   %s%s%s\n", ColorBold, ColorReset, ColorGreen, commit.Date, ColorReset)
		fmt.Printf("\n")
		fmt.Printf("    %s%s%s\n", ColorWhite, commit.Description, ColorReset)
		if i < len(newCommits)-1 {
			fmt.Printf("\n")
		}
	}

	fmt.Printf("\n")
}

func updateCommitNumFile(filePath string, commitNum int) {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	os.WriteFile(filePath, []byte(strconv.Itoa(commitNum)), 0644)
}

func joinNames(name []string) string {
	if len(name) == 0 {
		return ""
	}
	if len(name) == 1 {
		return name[0]
	}

	result := name[0]
	for i := 1; i < len(name); i++ {
		result += ", " + name[i]
	}
	return result
}

func shuffle(arr []string) []string {
	n := len(arr)
	for i := n - 1; i > 0; i-- {
		j := rand.Intn(i + 1)
		arr[i], arr[j] = arr[j], arr[i]
	}
	return arr
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
