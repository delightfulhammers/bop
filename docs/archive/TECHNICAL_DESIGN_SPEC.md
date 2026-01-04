# **Multi-LLM Code Review CLI \- Technical Design Specification**

Version: 1.0  
Date: 2025-10-20  
Status: Final

## **1\. Introduction**

This document provides a detailed technical design for the Multi-LLM Code Review CLI. It elaborates on the components defined in the Architecture Document, specifying data structures, interfaces, and logic for implementation in Go.

## **2\. Configuration System (internal/config)**

Configuration will be managed by Viper, loaded in the following priority order: CLI flags \> Environment variables \> Local .bop.yaml \> Global \~/.config/bop/config.yaml \> Defaults.

### **2.1. Core Configuration Structs**

// internal/config/config.go

// Config is the top-level configuration struct.  
type Config struct {  
	Providers   map\[string\]ProviderConfig \`yaml:"providers"\`  
	Merge       MergeConfig               \`yaml:"merge"\`  
	Git         GitConfig                 \`yaml:"git"\`  
	Output      OutputConfig              \`yaml:"output"\`  
	Budget      BudgetConfig              \`yaml:"budget"\`  
	Redaction   RedactionConfig           \`yaml:"redaction"\`  
	Determinism DeterminismConfig         \`yaml:"determinism"\`  
}

type ProviderConfig struct {  
	Enabled bool   \`yaml:"enabled"\`  
	Model   string \`yaml:"model"\` // Must be a specific, versioned model ID  
	APIKey  string \`yaml:"apiKey"\`  // Loaded from env var  
}

type MergeConfig struct {  
	Enabled  bool              \`yaml:"enabled"\`  
	Provider string            \`yaml:"provider"\`  
	Model    string            \`yaml:"model"\`  
	Strategy string            \`yaml:"strategy"\` // "consensus" or "ranked"  
	Weights  map\[string\]float64\`yaml:"weights"\`  
}

type BudgetConfig struct {  
	HardCapUSD        float64  \`yaml:"hardCapUSD"\`  
	DegradationPolicy \[\]string \`yaml:"degradationPolicy"\` // e.g., \["trim\_context", "split\_units", "drop\_models"\]  
}

type RedactionConfig struct {  
	Enabled    bool     \`yaml:"enabled"\`  
	DenyGlobs  \[\]string \`yaml:"denyGlobs"\`  
	AllowGlobs \[\]string \`yaml:"allowGlobs"\`  
}

type DeterminismConfig struct {  
	Enabled     bool    \`yaml:"enabled"\`  
	Temperature float64 \`yaml:"temperature"\` // Should be 0.0 for determinism  
	UseSeed     bool    \`yaml:"useSeed"\`  
}

## **3\. Git Engine (internal/git)**

Uses go-git for native Git operations. Its primary responsibility is to produce a single cumulative diff.

### **3.1. Interface and Data Structures**

// internal/git/engine.go

type Engine interface {  
	// GetCumulativeDiff resolves a scope (branch, commit range) to a single diff.  
	GetCumulativeDiff(ctx context.Context, scope string) (\*Diff, error)  
}

// Diff represents the complete set of changes for a review.  
type Diff struct {  
	FromCommitHash string  
	ToCommitHash   string  
	Files          \[\]FileDiff  
}

type FileDiff struct {  
	Path   string  
	Status string // "added", "modified", "deleted"  
	Patch  string // The raw unified diff patch for the file  
}

## **4\. LLM Provider (internal/llm)**

An interface-based design allows for easy addition of new LLM providers.

### **4.1. Core Interface**

// internal/llm/provider.go

type Provider interface {  
	Name() string  
	// Review performs a code review, accepting a deterministic seed.  
	Review(ctx context.Context, req ReviewRequest) (\*Review, error)  
}

type ReviewRequest struct {  
	Prompt   string  
	Seed     uint64 // For deterministic requests  
	MaxTokens int  
	// ... other provider-specific options  
}

// Review is the structured output from a single LLM.  
type Review struct {  
	ProviderName string    \`json:"providerName"\`  
	ModelName    string    \`json:"modelName"\`  
	Summary      string    \`json:"summary"\`  
	Findings     \[\]Finding \`json:"findings"\`  
}

// Finding represents a single issue identified by an LLM.  
type Finding struct {  
	ID          string  \`json:"id"\` // Hash of file+span+description for de-duplication  
	File        string  \`json:"file"\`  
	LineStart   int     \`json:"lineStart"\`  
	LineEnd     int     \`json:"lineEnd"\`  
	Severity    string  \`json:"severity"\` // "critical", "high", "medium", "low"  
	Category    string  \`json:"category"\` // "security", "performance", "style", etc.  
	Description string  \`json:"description"\`  
	Suggestion  string  \`json:"suggestion"\`  
	Evidence    bool    \`json:"evidence"\` // Did the model cite specific code lines?  
}

**Prompt Engineering:** Prompts will be engineered to request structured JSON output matching the Review struct. A System prompt will define the expert persona and task, while the User prompt will contain the redacted code diff and context.

## **5\. Persistence Layer (internal/store)**

Uses cgo-free SQLite for local storage.

### **5.1. Database Schema**

* **runs(run\_id, timestamp, scope, config\_hash, total\_cost)**  
* **reviews(review\_id, run\_id, provider, model)**  
* **findings(finding\_id, review\_id, finding\_hash, file, category, severity, description)**  
* **feedback(feedback\_id, finding\_id, status)**: status is "accepted" or "rejected".  
* **precision\_priors(provider, category, alpha, beta)**: Stores the parameters for the Beta distribution, representing model precision.

### **5.2. Store Interface**

// internal/store/store.go

type Store interface {  
	// Write methods  
	CreateRun(run Run) error  
	RecordFeedback(findingID string, status string) error

	// Read methods  
	GetPrecisionPriors() (map\[string\]map\[string\]float64, error)

	// Update methods  
	UpdatePrecisionPriors(feedback Feedback) error  
}

When feedback is recorded, the UpdatePrecisionPriors method updates the alpha (accepts) and beta (rejects) values for the given provider and category, recalibrating the model's trustworthiness.

## **6\. Merger Service (internal/merger)**

This service synthesizes multiple reviews into one.

### **6.1. Merging Logic**

1. **Fetch Priors:** Before merging, fetch the latest precision scores from the Store. precision \= alpha / (alpha \+ beta).  
2. **Group Findings:** Group all findings from all reviews by their finding\_hash (a hash of the file, line range, and a normalized description).  
3. **Score & Rank:** For each group (i.e., each unique issue), calculate a final score using the configured weights.  
   * score \= (w1 \* agreement) \+ (w2 \* avg\_severity) \+ (w3 \* max\_precision) \+ ...  
   * agreement: Number of models that found this issue.  
   * avg\_severity: The average severity assigned.  
   * max\_precision: The highest precision score among the models that found this issue.  
4. **Resolve Contradictions:** If models provide conflicting suggestions for the same issue, the suggestion from the model with the highest precision prior for that category is chosen.  
5. **Generate Merged Report:** Create a final MergedReview struct containing the ranked and de-duplicated list of findings.

## **7\. Output Manager (internal/output)**

Handles the generation of review files.

### **7.1. Supported Formats**

* **Markdown (.md):** Human-readable format, ideal for local review.  
* **JSON (.json):** Machine-readable format containing all raw data from individual and merged reviews.  
* **SARIF (.sarif):** Standardized format for static analysis tools. Findings will be mapped to the SARIF result schema. This enables direct integration with GitHub's Code Scanning alerts.

### **7.2. Directory Structure**

\<output\_dir\>/  
└── \<repo\_name\>\_\<branch\_name\>/  
    └── \<run\_timestamp\>/  
        ├── review-openai.md  
        ├── review-anthropic.json  
        ├── merged-review.md  
        ├── merged-review.sarif  
        └── metadata.json

## **8\. Redaction Engine (internal/redaction)**

A non-negotiable step in the workflow.

### **8.1. Implementation Strategy**

1. **Glob Matching:** First, check files against deny\_globs and allow\_globs from the config.  
2. **Regex Scanning:** Use a curated list of regular expressions to find common secret formats (API keys, private keys, etc.).  
3. **Entropy Analysis:** For each line, calculate the Shannon entropy. Strings with unusually high entropy are likely to be randomly generated secrets.  
4. **Replacement:** Replace found secrets with stable placeholders (e.g., \<REDACTED:ENTROPY\_1\>). A temporary, in-memory map is kept to reverse this process for local display only; the map is never persisted or sent externally.
