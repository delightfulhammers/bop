# **Multi-LLM Code Review CLI \- Architecture Document**

Version: 1.0  
Date: 2025-10-20  
Status: Final

## **1\. Overview**

This document outlines the high-level architecture for a sophisticated command-line interface (CLI) tool that leverages multiple Large Language Models (LLMs) to perform high-quality, parallel code reviews. The system is designed to be a secure, deterministic, and intelligent assistant for developers, integrable into both local workflows and automated CI/CD pipelines.

The core principles guiding this architecture are:

* **Modularity:** Components are designed with clear responsibilities and loosely coupled interfaces, facilitating independent development, testing, and maintenance.  
* **Extensibility:** The system can be easily extended with new LLM providers, output formats, or analysis techniques without major refactoring.  
* **Security:** A "security-first" approach ensures that sensitive data (code, secrets) is handled safely, with redaction being a non-negotiable, default step.  
* **Reproducibility:** The system is designed to be deterministic, ensuring that the same review on the same code with the same configuration yields identical results.  
* **Intelligence:** The tool incorporates a feedback loop, allowing it to learn from user interactions and improve the quality of its analysis over time.

## **2\. Architectural Diagram**

The system is composed of several key modules that work in concert to process a review request.

\+--------------------------------------------------------------------------+  
|                            User Interface Layer                          |  
|  \+---------------------------+       \+---------------------------------+ |  
|  |   CLI (Go \+ Cobra)        |       |    TUI (Bubble Tea)             | |  
|  | \- Command/Flag Parsing    |       | \- Interactive Mode              | |  
|  | \- Headless Operation      |       | \- Feedback Capture (Accept/Reject)| |  
|  \+-------------^-------------+       \+----------------^----------------+ |  
\+----------------|---------------------------------------|------------------+  
                 |                                       |  
\+----------------V---------------------------------------V------------------+  
|                            Application Core                              |  
|                                                                          |  
|  \+---------------------------+       \+---------------------------------+ |  
|  |   Config Manager (Viper)  |------\>|      Review Orchestrator        |\<----+  
|  | \- Layered Config Loading  |       | \- Manages LLM Provider Pool     |     |  
|  | \- Budget/Degradation Policy|       | \- Parallel Execution (Goroutines) |     |  
|  \+---------------------------+       | \- Deterministic Seeding         |     |  
|                                      \+------------------^----------------+     |  
|  \+---------------------------+                          |                      |  
|  |   Git Engine (go-git)     |       \+------------------V----------------+     |  
|  | \- Commit Scope Resolution |------\>|      Redaction Engine           |     |  
|  | \- Cumulative Diff Gen     |       | \- Regex & Entropy Scanning      |     |  
|  \+---------------------------+       | \- Secret Redaction (Default On)   |     |  
|                                      \+------------------V----------------+     |  
|                                                         |                      |  
|                                      \+------------------V----------------+     |  
|                                      |       Context Builder           |     |  
|                                      | \- Assembles Docs & User Prompts |     |  
|                                      \+------------------^----------------+     |  
|                                                         |                      |  
\+--------------------------------------------------------------------------+  
                 |                                       |  
\+----------------V----------------+      \+---------------V-----------------+  
|      Persistence Layer          |      |         Analysis Layer          |  
| \+-----------------------------+ |      | \+-----------------------------+ |  
| |      SQLite Store           | |      | |     Merger Service          | |  
| | \- Stores Run History, Costs |\<------\>| | \- Fetches Precision Priors    | |  
| | \- Manages Feedback          | |      | | \- Ranks & Synthesizes Findings| |  
| | \- Calculates Precision Priors | |      | \+-----------------------------+ |  
| \+-----------------------------+ |      \+---------------------------------+  
\+---------------------------------+  
                 |  
\+----------------V-----------------+  
|          Output Layer            |  
|  \+-----------------------------+ |  
|  |      Output Manager         | |  
|  | \- Markdown, JSON, SARIF     | |  
|  | \- Writes to Filesystem      | |  
|  \+-----------------------------+ |  
\+----------------------------------+

## **3\. Component Responsibilities**

### **3.1. User Interface Layer**

* **CLI (Cobra):** The primary entry point. Parses commands and flags for headless (e.g., CI/CD) operation.  
* **TUI (Bubble Tea):** Provides a rich, interactive terminal experience for local use. Critically, it includes the interface for capturing user feedback on the validity of LLM findings.

### **3.2. Application Core**

* **Config Manager (Viper):** Loads and merges configuration from multiple sources (files, env vars, flags). It is responsible for interpreting the budget and the graceful degradation policy.  
* **Git Engine (go-git):** Interacts with the Git repository. Its main job is to resolve the user's requested scope (e.g., branch X, commit Y..Z) into a single, cumulative diff.  
* **Redaction Engine:** A security-critical component that acts as a filter. It scans all code and context for secrets before they are passed to any other part of the system or an external API.  
* **Context Builder:** Assembles the final prompt payload by combining the redacted code diff with documentation, user-provided instructions, and other relevant context.  
* **Review Orchestrator:** The heart of the tool. It manages a pool of LLM provider workers, dispatches review jobs to run concurrently, applies deterministic seeding for reproducibility, and enforces the budget by triggering the degradation policy when necessary.

### **3.3. Persistence Layer**

* **SQLite Store:** The system's "memory." It stores the results of every review, including the findings, costs, and metadata. It also records user feedback (accepted/rejected), which is used to calculate and update precision scores (priors) for each LLM.

### **3.4. Analysis Layer**

* **Merger Service:** The intelligence hub for synthesis. After individual reviews are complete, this service queries the SQLite store for the latest precision priors. It uses these scores as weights to intelligently merge, de-duplicate, and rank the findings from all LLMs into a single, actionable report.

### **3.5. Output Layer**

* **Output Manager:** Responsible for generating the final review artifacts. It supports multiple output formats—**Markdown** for human readability, **JSON** for programmatic use, and **SARIF** for seamless integration with CI/CD platforms and code analysis tools.

## **4\. Data Flow**

1. A user initiates a review via the **CLI** or **TUI**.  
2. The **Config Manager** loads the full configuration.  
3. The **Git Engine** produces a cumulative diff for the requested scope.  
4. The **Redaction Engine** sanitizes the diff and any context files.  
5. The **Context Builder** prepares the prompt.  
6. The **Orchestrator** checks the budget, applies degradation if needed, and launches parallel review jobs to the configured LLM providers.  
7. Each LLM returns a structured review, which is collected by the Orchestrator.  
8. The **Merger Service** fetches precision priors from the **SQLite Store**, then synthesizes the individual reviews into a final merged report.  
9. The **Output Manager** writes the final reports to the filesystem.  
10. The run results and findings are persisted to the **SQLite Store**. If in TUI mode, the user can now provide feedback, which updates the precision priors in the database.