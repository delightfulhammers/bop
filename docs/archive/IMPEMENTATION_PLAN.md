# **Multi-LLM Code Review CLI \- Implementation Plan**

Version: 1.0  
Date: 2025-10-20  
Status: Final

## **1\. Overview**

This document outlines a phased implementation plan for the Multi-LLM Code Review CLI. The project is broken down into four main phases, starting with a core functional MVP and progressively layering on more advanced features. This approach allows for early delivery of value and iterative development based on feedback.

## **2\. Phase 1: Core Engine & Single Provider MVP (Weeks 1-3)**

**Goal:** Establish a working end-to-end review pipeline with a single LLM provider. The focus is on the core mechanics, not the advanced features.

### **Deliverables:**

* **\[x\] Project Scaffolding:** Set up Go project structure, CI with GitHub Actions, and linting.  
* **\[x\] CLI Framework (Cobra/Viper):** Implement basic review branch command and layered configuration loading.  
* **\[x\] Git Engine:** Implement GetCumulativeDiff for branch-based reviews.  
* **\[x\] LLM Provider Interface:** Define the Provider interface.  
* **\[x\] Single Provider Implementation:** Implement a provider for one service (e.g., OpenAI or Anthropic).  
* **\[x\] Basic Orchestrator:** A simple orchestrator that calls the single provider.  
* **\[x\] Markdown Output:** Implement the OutputManager with support for Markdown only.  
* **\[x\] Core Data Structs:** Define all key structs for Review, Finding, etc.

**Outcome:** A user can run bop review branch main and get a single Markdown review file in an output directory. Reproducible automation is provided through Mage targets (`mage ci`).

## **3\. Phase 2: Parallelism, Merging & Determinism (Weeks 4-6)**

**Goal:** Introduce the multi-LLM capability, intelligent merging, and ensure reproducible results.

### **Deliverables:**

* **\[x\] Additional Providers:** Implement at least two more LLM providers (Anthropic/Claude, Google Gemini, Ollama for local support).
* **\[x\] Parallel Orchestrator:** Upgrade the orchestrator to run all enabled providers concurrently using goroutines and channels.
* **\[x\] Basic Merger Service:** Implement a simple merger that de-duplicates findings based on a content hash.
* **\[x\] Determinism Engine:** Integrate deterministic seeding into the orchestrator and Provider interface. Set temperature=0.0 in API calls.
* **\[x\] JSON & SARIF Output:** Extend the OutputManager to support JSON and SARIF formats.
* **\[x\] Redaction Engine (v1):** Implement regex-based secret redaction.

**Outcome:** The tool can now run 4 models in parallel (OpenAI, Anthropic, Gemini, Ollama), produce a de-duplicated merged review, and generate identical output for identical inputs. CI integration via SARIF is now possible.

## **4\. Phase 3: Intelligence & Feedback Loop (Weeks 7-9)**

**Goal:** Make the tool "smart" by introducing the local database and feedback mechanism.

### **Deliverables:**

* **\[x\] SQLite Store:** Set up the SQLite database schema and Store interface.
* **\[x\] Precision Priors:** Implement the logic in the Store to calculate and update precision scores (Beta distribution) based on user feedback.
* **\[x\] Persistence Logic:** Integrate the store to save all run history, findings, and metadata.
* **\[~\] HTTP API Clients:** Implement real HTTP clients for OpenAI, Anthropic, Gemini, Ollama (in progress).
* **\[ \] TUI (Bubble Tea):** Build the interactive TUI.
* **\[ \] Feedback Capture:** In the TUI, implement the ability for users to mark findings as accepted or rejected.
* **\[ \] Intelligent Merger (v2):** Upgrade the Merger Service to use the precision priors from the database to rank and weight findings during synthesis.
* **\[ \] Redaction Engine (v2):** Add entropy-based secret detection.

**Outcome:** The tool now learns from user feedback, improving the quality and relevance of its merged reviews over time. The interactive TUI provides a much richer user experience.

## **5\. Phase 4: Budgeting, Polish & Release (Weeks 10-12)**

**Goal:** Add production-grade features, comprehensive testing, and prepare for a v1.0 release.

### **Deliverables:**

* **\[ \] Budget & Degradation Engine:** Implement the cost estimation and graceful degradation logic in the orchestrator.  
* **\[ \] Comprehensive Testing:** Achieve \>80% unit test coverage. Add integration tests that use mock LLM servers.  
* **\[ \] Documentation:** Write thorough README, usage examples, and configuration guides.  
* **\[ \] TUI Enhancements:** Polish the TUI with better layouts, help screens, and progress indicators.  
* **\[ \] Release Automation:** Set up goreleaser to automate cross-platform binary builds and releases.  
* **\[ \] Beta Testing:** Distribute to a small group of users for feedback before the official release.

**Outcome:** A polished, secure, intelligent, and production-ready v1.0 release.

## **6\. Success Metrics**

* **Reproducibility:** A bit-for-bit identical output must be generated when running the same review with a fixed configuration.  
* **Performance:** For a medium-sized PR (\<1000 LoC change), a parallel review with 3 models should complete in under 2 minutes.  
* **Precision:** After a training period, the precision of the merged review (as measured by the accepted rate) should exceed 75%.  
* **Adoption:** The tool is successfully integrated into at least one automated CI workflow.  
* **Security:** Zero reported leaks of sensitive data from code sent for review.

## **7\. Future Enhancements (Post v1.0)**

* **RAG for Context:** Implement a local RAG pipeline (embedding documentation) for more intelligent context injection.  
* **GitHub App Integration:** Post reviews directly as PR comments or annotations.  
* **Plugin System:** Allow users to write their own analyzers or output formatters (e.g., via WASM).  
* **Caching:** Implement caching of LLM responses for unchanged code chunks to speed up subsequent reviews.
