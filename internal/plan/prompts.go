package plan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/templates"
)

const (
	docsDirName        = "_governator/docs"
	tasksDirName = "_governator/tasks"
	reasoningDirName   = "_governator/reasoning"
	templatesDirName   = "_governator/templates"
	workerContractPath = "_governator/worker-contract.md"
	planPromptMode     = 0o644
)

type Document struct {
	Path    string
	Content string
}

type agentSpec struct {
	Name     string
	Template string
	FileName string
	Role     index.Role
}

var agentSpecs = []agentSpec{
	{Name: "Architecture Baseline", Template: "planning/architecture-baseline.md", FileName: "architecture-baseline.md", Role: "architect"},
	{Name: "Gap Analysis", Template: "planning/gap-analysis.md", FileName: "gap-analysis.md", Role: "generalist"},
	{Name: "Roadmap Planning", Template: "planning/roadmap.md", FileName: "roadmap.md", Role: "planner"},
	{Name: "Task Planning", Template: "planning/tasks.md", FileName: "task-planning.md", Role: "planner"},
}

type promptProvider func(ctx promptContext, spec agentSpec) (string, error)

type promptSequenceEntry struct {
	ID       string
	Title    string
	Optional bool
	Provider promptProvider
}

var promptSequence = []promptSequenceEntry{
	{ID: "reasoning-effort", Title: "Reasoning guidance", Optional: true, Provider: reasoningProvider},
	{ID: "worker-contract", Title: "Worker contract", Provider: workerContractProvider},
	{ID: "role", Title: "Role instructions", Provider: roleProvider},
	{ID: "custom-global", Title: "Custom global prompts", Optional: true, Provider: customGlobalProvider},
	{ID: "custom-role", Title: "Custom role prompts", Optional: true, Provider: customRoleProvider},
	{ID: "task", Title: "Agent task template", Provider: taskProvider},
}

type promptContext struct {
	repoRoot   string
	cfg        config.Config
	override   string
	registry   roles.Registry
	projectDoc Document
	docPaths   []string
	taskPaths  []string
	warn       func(string)
}

func prepareAgentPrompts(repoRoot string, warn func(string), cfg config.Config, override string) ([]PromptInfo, string, error) {
	projectDoc, err := loadGovernatorDoc(repoRoot)
	if err != nil {
		return nil, "", err
	}

	registry, err := roles.LoadRegistry(repoRoot, warn)
	if err != nil {
		return nil, "", fmt.Errorf("load role registry: %w", err)
	}

	docPaths, err := collectRelativePaths(repoRoot, docsDirName)
	if err != nil {
		return nil, "", err
	}
	if len(docPaths) == 0 {
		emitWarning(warn, "no architecture artifacts found in _governator/docs")
	}

	taskPaths, err := collectRelativePaths(repoRoot, tasksDirName)
	if err != nil {
		return nil, "", err
	}
	if len(taskPaths) == 0 {
		emitWarning(warn, "no tasks found in _governator/tasks")
	}

	ctx := promptContext{
		repoRoot:   repoRoot,
		cfg:        cfg,
		override:   strings.TrimSpace(override),
		registry:   registry,
		projectDoc: projectDoc,
		docPaths:   docPaths,
		taskPaths:  taskPaths,
		warn:       warn,
	}

	promptDir := filepath.Join(repoRoot, localStateDirName, plannerStateDirName)
	if err := os.MkdirAll(promptDir, plannerStateMode); err != nil {
		return nil, "", fmt.Errorf("create planner state dir %s: %w", promptDir, err)
	}

	results := make([]PromptInfo, 0, len(agentSpecs))
	for _, spec := range agentSpecs {
		content, err := buildPromptForAgent(ctx, spec)
		if err != nil {
			return nil, "", err
		}
		path := filepath.Join(promptDir, spec.FileName)
		if err := os.WriteFile(path, []byte(content), planPromptMode); err != nil {
			return nil, "", fmt.Errorf("write agent prompt %s: %w", path, err)
		}
		results = append(results, PromptInfo{
			Agent: spec.Name,
			Path:  repoRelativePath(repoRoot, path),
		})
	}

	return results, promptDir, nil
}

func buildPromptForAgent(ctx promptContext, spec agentSpec) (string, error) {
	sections := make([]promptSection, 0, len(promptSequence))
	for _, entry := range promptSequence {
		content, err := entry.Provider(ctx, spec)
		if err != nil {
			return "", fmt.Errorf("provider %s: %w", entry.ID, err)
		}
		content = strings.TrimSpace(content)
		if content == "" {
			if entry.Optional {
				continue
			}
			return "", fmt.Errorf("%s is required for agent %s", entry.Title, spec.Name)
		}
		sections = append(sections, promptSection{
			Title:   entry.Title,
			Content: content,
		})
	}
	return renderPrompt(spec.Name, sections), nil
}

type promptSection struct {
	Title   string
	Content string
}

func renderPrompt(agentName string, sections []promptSection) string {
	builder := strings.Builder{}
	builder.WriteString("# Agent: ")
	builder.WriteString(agentName)
	builder.WriteString("\n\n")
	for _, section := range sections {
		builder.WriteString("## ")
		builder.WriteString(section.Title)
		builder.WriteString("\n")
		builder.WriteString(section.Content)
		builder.WriteString("\n\n")
	}
	return strings.TrimSpace(builder.String()) + "\n"
}

func reasoningProvider(ctx promptContext, spec agentSpec) (string, error) {
	level := ctx.reasoningEffortFor(spec)
	if level == "" {
		return "", nil
	}
	path := filepath.Join(ctx.repoRoot, reasoningDirName, level+".md")
	content, ok, err := readOptionalFile(path)
	if err != nil {
		return "", err
	}
	if !ok {
		emitWarning(ctx.warn, fmt.Sprintf("missing reasoning effort prompt %s", path))
		return "", nil
	}
	return content, nil
}

func (ctx promptContext) reasoningEffortFor(spec agentSpec) string {
	if ctx.override != "" {
		return ctx.override
	}
	role := strings.TrimSpace(string(spec.Role))
	return ctx.cfg.ReasoningEffort.LevelForRole(role)
}

func workerContractProvider(ctx promptContext, spec agentSpec) (string, error) {
	content, ok, err := readOptionalFile(filepath.Join(ctx.repoRoot, workerContractPath))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("%s is required", workerContractPath)
	}
	return content, nil
}

func roleProvider(ctx promptContext, spec agentSpec) (string, error) {
	path, ok := ctx.registry.RolePromptPath(spec.Role)
	if !ok {
		return "", fmt.Errorf("missing role prompt for %s", spec.Role)
	}
	return readFileContent(ctx.repoRoot, path)
}

func customGlobalProvider(ctx promptContext, spec agentSpec) (string, error) {
	path, ok := ctx.registry.CustomGlobalPromptPath()
	if !ok {
		return "", nil
	}
	return readFileContent(ctx.repoRoot, path)
}

func customRoleProvider(ctx promptContext, spec agentSpec) (string, error) {
	path, ok := ctx.registry.CustomRolePromptPath(spec.Role)
	if !ok {
		return "", nil
	}
	return readFileContent(ctx.repoRoot, path)
}

func taskProvider(ctx promptContext, spec agentSpec) (string, error) {
	template, err := loadPlanningTemplate(ctx.repoRoot, spec.Template)
	if err != nil {
		return "", err
	}
	builder := strings.Builder{}
	builder.WriteString(template)
	builder.WriteString("\n\n")
	builder.WriteString("## Project intent (")
	builder.WriteString(ctx.projectDoc.Path)
	builder.WriteString(")\n")
	builder.WriteString(ctx.projectDoc.Content)
	builder.WriteString("\n\n")
	builder.WriteString("## Available architecture artifacts\n")
	if len(ctx.docPaths) == 0 {
		builder.WriteString("- (none)\n")
	} else {
		for _, path := range ctx.docPaths {
			builder.WriteString("- ")
			builder.WriteString(path)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n## Existing tasks\n")
	if len(ctx.taskPaths) == 0 {
		builder.WriteString("- (none)\n")
	} else {
		for _, path := range ctx.taskPaths {
			builder.WriteString("- ")
			builder.WriteString(path)
			builder.WriteString("\n")
		}
	}
	return builder.String(), nil
}

func loadGovernatorDoc(repoRoot string) (Document, error) {
	path := filepath.Join(repoRoot, "GOVERNATOR.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return Document{}, fmt.Errorf("%s is empty", path)
	}
	return Document{
		Path:    "GOVERNATOR.md",
		Content: content,
	}, nil
}

func collectRelativePaths(repoRoot string, relativeDir string) ([]string, error) {
	dir := filepath.Join(repoRoot, relativeDir)
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat directory %s: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	var paths []string
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel := repoRelativePath(repoRoot, path)
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	sort.Strings(paths)
	return paths, nil
}

func loadPlanningTemplate(repoRoot string, name string) (string, error) {
	if err := validateTemplateName(name); err != nil {
		return "", err
	}
	localPath := filepath.Join(repoRoot, templatesDirName, filepath.FromSlash(name))
	info, err := os.Stat(localPath)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("template path is a directory: %s", localPath)
		}
		data, readErr := os.ReadFile(localPath)
		if readErr != nil {
			return "", fmt.Errorf("read template %s: %w", localPath, readErr)
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			return "", fmt.Errorf("template %s is empty", localPath)
		}
		return content, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("stat template %s: %w", localPath, err)
	}

	data, err := templates.Read(name)
	if err != nil {
		return "", err
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", fmt.Errorf("template %s is empty", name)
	}
	return content, nil
}

func readFileContent(repoRoot, relativePath string) (string, error) {
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return "", fmt.Errorf("%s is empty", path)
	}
	return content, nil
}

func readOptionalFile(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("read %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

func emitWarning(warn func(string), message string) {
	if warn == nil || strings.TrimSpace(message) == "" {
		return
	}
	warn(message)
}

func validateTemplateName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("template name is required")
	}
	if strings.Contains(trimmed, "\\") {
		return errors.New("template name must use forward slashes")
	}
	if strings.HasPrefix(trimmed, "/") {
		return errors.New("template name must be relative")
	}
	segments := strings.Split(trimmed, "/")
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("template name contains invalid segments")
		}
	}
	if !strings.HasPrefix(trimmed, "planning/") {
		return errors.New("template name must start with planning/")
	}
	return nil
}
