package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// InputItem represents the parsed command-line item
type InputItem struct {
	Name            string
	Path            string
	Stage           string
	Type            string
	URL             string
	ScriptDoNotWait string
	PkgSkipIf       string
	Retries         string
	RetryWait       string
	Required        string
}

// JSONItem represents an item in the final JSON output
type JSONItem struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	File string `json:"file"`
	Hash string `json:"hash"`
	Type string `json:"type"`

	// Package-specific fields
	PackageID string `json:"packageid,omitempty"`
	Version   string `json:"version,omitempty"`
	SkipIf    string `json:"skip_if,omitempty"`

	// Script-specific fields
	DoNotWait bool `json:"donotwait,omitempty"`

	// Common optional fields
	PkgRequired bool `json:"pkg_required,omitempty"`
	Retries     int  `json:"retries,omitempty"`
	RetryWait   int  `json:"retrywait,omitempty"`
}

// JSONOutput represents the final JSON structure that will be written to file
type JSONOutput struct {
	Preflight      []JSONItem `json:"preflight"`
	SetupAssistant []JSONItem `json:"setupassistant"`
	Userland       []JSONItem `json:"userland"`
}

// ItemList is a custom type that implements flag.Value interface for the --item flag
type ItemList []InputItem

// String implements the flag.Value interface
func (i *ItemList) String() string {
	return fmt.Sprintf("%v", *i)
}

// Set implements the flag.Value interface
func (i *ItemList) Set(value string) error {
	parts := strings.Fields(value)
	if len(parts) != 10 {
		return fmt.Errorf("item must have exactly 10 key=value pairs, got %d", len(parts))
	}

	item := InputItem{}

	// Parse each key=value pair
	for _, part := range parts {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return fmt.Errorf("invalid key=value format: %s", part)
		}

		key, value := kv[0], kv[1]

		switch key {
		case "item-name":
			item.Name = value
		case "item-path":
			item.Path = value
		case "item-stage":
			item.Stage = value
		case "item-type":
			item.Type = value
		case "item-url":
			item.URL = value
		case "script-do-not-wait":
			item.ScriptDoNotWait = value
		case "pkg-skip-if":
			item.PkgSkipIf = value
		case "retries":
			item.Retries = value
		case "retrywait":
			item.RetryWait = value
		case "required":
			item.Required = value
		default:
			return fmt.Errorf("unknown item key: %s", key)
		}
	}

	*i = append(*i, item)
	return nil
}

func main() {
	baseURL := flag.String("base-url", "", "Base URL to where root dir is hosted")
	output := flag.String("output", "", "Required: Output directory for the generated json file")
	compat := flag.Bool("compat", false, "Generate paths using original InstallApplications layout (/Library/installapplications)")
	installPathFlag := flag.String("install-path", "", "Override base install path used for scripts/packages (default: /Library/go-installapplications; ignored if --compat is set)")

	var items ItemList
	flag.Var(&items, "item", "Required: Options for item. Format: item-name=NAME item-path=PATH item-stage=STAGE item-type=TYPE item-url=URL script-do-not-wait=BOOL pkg-skip-if=ARCH retries=INT retrywait=INT required=BOOL")

	flag.Parse()

	if *output == "" {
		log.Fatal("output is required")
	}

	if *baseURL == "" {
		log.Fatal("base-url is required")
	}

	if len(items) == 0 {
		log.Fatal("at least one --item is required")
	}

	// Determine base install path behavior
	baseInstallPath := "/Library/go-installapplications"
	if *compat {
		if *installPathFlag != "" {
			log.Fatal("--compat cannot be used together with --install-path; choose one")
		}
		baseInstallPath = "/Library/installapplications"
	} else if *installPathFlag != "" {
		baseInstallPath = *installPathFlag
	}

	fmt.Printf("Base URL: %s\n", *baseURL)
	fmt.Printf("Output: %s\n", *output)
	fmt.Printf("Items (%d):\n", len(items))
	for i, item := range items {
		fmt.Printf("  Item %d: %+v\n", i+1, item)
	}

	stages := buildItemDict(items, *baseURL, baseInstallPath)

	jsonData, err := json.MarshalIndent(stages, "", "  ")
	if err != nil {
		log.Fatalf("Error marshaling JSON: %v", err)
	}

	savePath := filepath.Join(*output, "bootstrap.json")
	err = os.WriteFile(savePath, jsonData, 0644)
	if err != nil {
		log.Fatalf("Error writing JSON file to %s: %v", savePath, err)
	}

	fmt.Printf("Json saved to %s\n", savePath)
}

func buildItemDict(items ItemList, baseURL string, baseInstallPath string) JSONOutput {
	// Initialize the output structure
	output := JSONOutput{
		Preflight:      []JSONItem{},
		SetupAssistant: []JSONItem{},
		Userland:       []JSONItem{},
	}

	// Process each input item
	for _, inputItem := range items {
		jsonItem := JSONItem{}

		fileExt := filepath.Ext(inputItem.Path)
		fileName := filepath.Base(inputItem.Path)
		filePath := inputItem.Path

		if inputItem.Type != "package" && inputItem.Type != "rootscript" && inputItem.Type != "rootfile" && inputItem.Type != "userscript" && inputItem.Type != "userfile" {
			fmt.Printf("Invalid type: %s for %s\n", inputItem.Type, filePath)
			os.Exit(1)
		}

		jsonItem.Type = inputItem.Type

		// Ensure the type is set correctly for packages
		if fileExt == ".pkg" {
			jsonItem.Type = "package"
		}

		if inputItem.Stage == "" {
			inputItem.Stage = "userland"
		}
		if inputItem.Stage != "preflight" && inputItem.Stage != "setupassistant" && inputItem.Stage != "userland" {
			fmt.Printf("Invalid stage: %s for %s\n", inputItem.Stage, filePath)
			os.Exit(1)
		}

		if inputItem.URL == "" {
			jsonItem.URL = fmt.Sprintf("%s/%s/%s", baseURL, inputItem.Stage, fileName)
		} else {
			jsonItem.URL = inputItem.URL
		}

		if inputItem.Name == "" {
			jsonItem.Name = fileName
		} else {
			jsonItem.Name = inputItem.Name
		}

		jsonItem.Hash = getHash(filePath)

		if inputItem.Type == "rootscript" || inputItem.Type == "userscript" {
			if inputItem.Type == "userscript" {
				jsonItem.File = filepath.Join(baseInstallPath, "userscripts", fileName)
			} else {
				jsonItem.File = filepath.Join(baseInstallPath, fileName)
			}

			jsonItem.DoNotWait = false
			switch inputItem.ScriptDoNotWait {
			case "true", "True", "1", "yes", "y", "false", "False", "0", "no", "n":
				if inputItem.ScriptDoNotWait == "true" || inputItem.ScriptDoNotWait == "True" || inputItem.ScriptDoNotWait == "1" || inputItem.ScriptDoNotWait == "yes" || inputItem.ScriptDoNotWait == "y" {
					jsonItem.DoNotWait = true
				}
			default:
				fmt.Printf("Invalid script-do-not-wait: %s for %s\n", inputItem.ScriptDoNotWait, filePath)
				os.Exit(1)
			}
		}

		if inputItem.Type == "package" {
			pkgId, pkgVersion := getPkgInfo(filePath)
			jsonItem.File = filepath.Join(baseInstallPath, fileName)
			jsonItem.PackageID = pkgId
			jsonItem.Version = pkgVersion

			if inputItem.PkgSkipIf != "false" && inputItem.PkgSkipIf != "False" && inputItem.PkgSkipIf != "0" && inputItem.PkgSkipIf != "no" && inputItem.PkgSkipIf != "n" && inputItem.PkgSkipIf != "" {
				switch inputItem.PkgSkipIf {
				case "intel", "arm64", "x86_64", "apple_silicon":
					jsonItem.SkipIf = inputItem.PkgSkipIf
				default:
					fmt.Printf("Invalid pkg-skip-if: %s for %s\n", inputItem.PkgSkipIf, filePath)
					os.Exit(1)
				}
			}

			// Handle the pkg_required field (input key is 'required' for IA compatibility)
			jsonItem.PkgRequired = false
			if inputItem.Required != "false" && inputItem.Required != "False" && inputItem.Required != "0" && inputItem.Required != "no" && inputItem.Required != "n" && inputItem.Required != "" {
				switch inputItem.Required {
				case "true", "True", "1", "yes", "y":
					jsonItem.PkgRequired = true
				default:
					fmt.Printf("Invalid required: %s for %s\n", inputItem.Required, filePath)
					os.Exit(1)
				}
			}
		}

		// rootfile/userfile: file is a destination path as provided by item-path
		if inputItem.Type == "rootfile" || inputItem.Type == "userfile" {
			jsonItem.File = inputItem.Path
		}

		if inputItem.Retries != "" {
			retries, err := strconv.Atoi(inputItem.Retries)
			if err != nil {
				fmt.Printf("Invalid retries value: %s for %s\n", inputItem.Retries, filePath)
				os.Exit(1)
			}
			jsonItem.Retries = retries
		}
		if inputItem.RetryWait != "" {
			retryWait, err := strconv.Atoi(inputItem.RetryWait)
			if err != nil {
				fmt.Printf("Invalid retry-wait value: %s for %s\n", inputItem.RetryWait, filePath)
				os.Exit(1)
			}
			jsonItem.RetryWait = retryWait
		}

		// Add to appropriate stage
		switch inputItem.Stage {
		case "preflight":
			output.Preflight = append(output.Preflight, jsonItem)
		case "setupassistant":
			output.SetupAssistant = append(output.SetupAssistant, jsonItem)
		case "userland":
			output.Userland = append(output.Userland, jsonItem)
		default:
			// Default to userland if stage is not specified or invalid
			output.Userland = append(output.Userland, jsonItem)
		}
	}

	return output
}

func getHash(filePath string) string {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("FILE NOT FOUND - CHECK YOUR PATH: %s\n", filePath)
		return "FILE NOT FOUND - CHECK YOUR PATH"
	}

	file, err := os.Open(filePath)
	if err != nil {
		fmt.Printf("Error opening file %s: %v\n", filePath, err)
		return "ERROR OPENING FILE"
	}
	defer file.Close()

	hasher := sha256.New()

	_, err = io.Copy(hasher, file)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", filePath, err)
		return "ERROR READING FILE"
	}

	hashBytes := hasher.Sum(nil)
	return hex.EncodeToString(hashBytes)
}

type PackageInfo struct {
	XMLName    xml.Name `xml:"pkg-info"`
	Identifier string   `xml:"identifier,attr"`
	Version    string   `xml:"version,attr"`
}

func getPkgInfo(filePath string) (string, string) {
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		fmt.Printf("Package file not found: %s\n", filePath)
		return "", ""
	}

	pkgInfoPath, err := getPkgInfoPath(filePath)
	if err != nil {
		fmt.Printf("Error getting PackageInfo path: %v\n", err)
		return "", ""
	}

	extractedPath, err := extractPackageInfo(filePath, pkgInfoPath)
	if err != nil {
		fmt.Printf("Error extracting PackageInfo: %v\n", err)
		return "", ""
	}
	defer os.Remove(extractedPath)

	xmlData, err := os.ReadFile(extractedPath)
	if err != nil {
		fmt.Printf("Error reading PackageInfo XML: %v\n", err)
		return "", ""
	}

	var pkgInfo PackageInfo
	err = xml.Unmarshal(xmlData, &pkgInfo)
	if err != nil {
		fmt.Printf("Error parsing PackageInfo XML: %v\n", err)
		return "", ""
	}

	return pkgInfo.Identifier, pkgInfo.Version
}

func getPkgInfoPath(filePath string) (string, error) {
	// Use xar to list the contents of the package
	cmd := exec.Command("/usr/bin/xar", "-tf", filePath)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("error running xar -tf: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PackageInfo") || strings.HasSuffix(line, ".pkg/PackageInfo") {
			return line, nil
		}
	}

	return "", fmt.Errorf("PackageInfo not found in package")
}

func extractPackageInfo(pkgPath, pkgInfoPath string) (string, error) {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "pkginfo-*")
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %v", err)
	}

	cmd := exec.Command("/usr/bin/xar", "-xf", pkgPath, "-C", tmpDir, pkgInfoPath)
	err = cmd.Run()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("error extracting PackageInfo: %v", err)
	}

	extractedPath := filepath.Join(tmpDir, pkgInfoPath)
	return extractedPath, nil
}
