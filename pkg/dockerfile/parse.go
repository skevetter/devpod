package dockerfile

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/instructions"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
	"github.com/moby/buildkit/frontend/dockerfile/shell"
	"github.com/skevetter/log/scanner"
)

var dockerfileSyntax = regexp.MustCompile(`(?m)^[\s\t]*#[\s\t]*syntax=.*$`)

func (d *Dockerfile) FindUserStatement(buildArgs, baseImageEnv map[string]string, target string) string {
	stage, ok := d.StagesByTarget[target]
	if !ok && len(d.Stages) > 0 {
		stage = d.Stages[len(d.Stages)-1]
	}

	seenStages := make(map[string]bool, 4)
	for stage != nil {
		stageKey := stageID(&stage.BaseStage)
		if seenStages[stageKey] {
			return ""
		}
		seenStages[stageKey] = true

		if len(stage.Users) > 0 {
			return d.replaceVariables(stage.Users[len(stage.Users)-1].Key, buildArgs, baseImageEnv, &stage.BaseStage, 0)
		}

		if stage.Image == "" {
			return ""
		}

		image := d.replaceVariables(stage.Image, buildArgs, baseImageEnv, &d.Preamble.BaseStage, d.Stages[0].Instructions[0].StartLine)
		stage, ok = d.StagesByTarget[image]
		if !ok {
			return ""
		}
	}
	return ""
}

func stageID(stage *BaseStage) string {
	return stage.Image + "-" + stage.Target
}

func (d *Dockerfile) FindBaseImage(buildArgs map[string]string, target string) string {
	stage := d.StagesByTarget[target]
	if stage == nil && len(d.Stages) > 0 {
		stage = d.Stages[len(d.Stages)-1]
	}
	if stage == nil {
		return ""
	}
	return d.replaceVariables(stage.Image, buildArgs, nil, &stage.BaseStage, 0)
}

func (d *Dockerfile) BuildContextFiles() []string {
	files := make([]string, 0, 8) // Pre-allocate for typical dockerfile
	for _, stage := range d.Stages {
		for _, in := range stage.Instructions {
			if cmd, err := instructions.ParseCommand(in); err == nil {
				if addCmd, ok := cmd.(*instructions.AddCommand); ok {
					files = append(files, addCmd.SourcePaths...)
				} else if copyCmd, ok := cmd.(*instructions.CopyCommand); ok {
					files = append(files, copyCmd.SourcePaths...)
				}
			}
		}
	}
	return files
}

var shellLexer = shell.NewLex('\\')

func (d *Dockerfile) replaceVariables(val string, buildArgs, baseImageEnv map[string]string, stage *BaseStage, _ int) string {
	result, _, err := shellLexer.ProcessWord(val, &envMap{d, buildArgs, baseImageEnv, stage, 0})
	if err != nil {
		return val
	}
	return result
}

type envMap struct {
	dockerfile   *Dockerfile
	buildArgs    map[string]string
	baseImageEnv map[string]string
	stage        *BaseStage
	_            int // unused untilLine field for compatibility
}

func (e *envMap) Get(key string) (string, bool) {
	return e.dockerfile.findValueWithFound(e.buildArgs, e.baseImageEnv, key, e.stage, 0)
}

func (e *envMap) Keys() []string {
	keys := make([]string, 0, len(e.buildArgs)+len(e.baseImageEnv))
	for k := range e.buildArgs {
		keys = append(keys, k)
	}
	for k := range e.baseImageEnv {
		keys = append(keys, k)
	}
	return keys
}

func (d *Dockerfile) findValueWithFound(buildArgs, baseImageEnv map[string]string, variable string, stage *BaseStage, _ int) (string, bool) {
	if buildArgs == nil {
		buildArgs = make(map[string]string)
	}

	seenStages := make(map[string]bool, 4) // Pre-allocate for typical multi-stage
	for {
		stageKey := stageID(stage)
		if seenStages[stageKey] {
			return "", false
		}
		seenStages[stageKey] = true

		// Search args (reverse order for precedence)
		for i := len(stage.Args) - 1; i >= 0; i-- {
			arg := &stage.Args[i]
			if arg.Key != variable {
				continue
			}
			if val := buildArgs[arg.Key]; val != "" {
				return strings.Trim(val, "\"'"), true
			}
			if arg.Value != nil && *arg.Value != "" {
				value := d.replaceVariables(*arg.Value, buildArgs, baseImageEnv, stage, 0)
				return strings.Trim(value, "\"'"), true
			}
			return "", true
		}

		// Search env (reverse order for precedence)
		for i := len(stage.Envs) - 1; i >= 0; i-- {
			env := &stage.Envs[i]
			if env.Key != variable {
				continue
			}
			if env.Value != "" {
				return d.replaceVariables(env.Value, buildArgs, baseImageEnv, stage, 0), true
			}
			return "", true
		}

		if stage == &d.Preamble.BaseStage {
			if baseImageEnv != nil {
				if value, exists := baseImageEnv[variable]; exists {
					return value, true
				}
			}
			return "", false
		}

		image := d.replaceVariables(stage.Image, buildArgs, baseImageEnv, &d.Preamble.BaseStage, d.Stages[0].Instructions[0].StartLine)
		if foundStage, ok := d.StagesByTarget[image]; ok {
			stage = &foundStage.BaseStage
		} else {
			stage = &d.Preamble.BaseStage
		}
	}
}

func RemoveSyntaxVersion(dockerfileContent string) string {
	return dockerfileSyntax.ReplaceAllString(dockerfileContent, "")
}

func EnsureDockerfileHasFinalStageName(dockerfileContent, defaultLastStageName string) (string, string, error) {
	result, err := parser.Parse(strings.NewReader(dockerfileContent))
	if err != nil {
		return "", "", err
	}

	var lastChild *parser.Node
	for _, child := range result.AST.Children {
		if strings.ToLower(child.Value) == command.From {
			lastChild = child
		}
	}

	if lastChild == nil {
		return "", "", fmt.Errorf("no FROM statement in dockerfile")
	}
	if lastChild.Next == nil {
		return "", "", fmt.Errorf("cannot parse FROM statement in dockerfile")
	}

	if lastChild.Next.Next != nil && lastChild.Next.Next.Next != nil && strings.EqualFold(lastChild.Next.Next.Value, "as") {
		return lastChild.Next.Next.Next.Value, "", nil
	}

	lastChild.Next.Next = &parser.Node{
		Value: "AS",
		Next:  &parser.Node{Value: defaultLastStageName},
	}
	return defaultLastStageName, ReplaceInDockerfile(dockerfileContent, lastChild), nil
}

func ReplaceInDockerfile(dockerfileContent string, node *parser.Node) string {
	scan := scanner.NewScanner(strings.NewReader(dockerfileContent))
	var lines []string
	for lineNumber := 1; scan.Scan(); lineNumber++ {
		if lineNumber >= node.StartLine && lineNumber <= node.EndLine {
			lines = append(lines, DumpNode(node))
		} else {
			lines = append(lines, scan.Text())
		}
	}
	return strings.Join(lines, "\n")
}

type Dockerfile struct {
	Raw string

	Directives []*parser.Directive
	Preamble   *Preamble
	Syntax     string // https://docs.docker.com/build/concepts/dockerfile/#dockerfile-syntax

	Stages         []*Stage
	StagesByTarget map[string]*Stage
}

type Preamble struct {
	BaseStage
}

type Stage struct {
	BaseStage
	Users []instructions.KeyValuePair
}

type BaseStage struct {
	Image  string
	Target string

	Envs         []instructions.KeyValuePair
	Args         []instructions.KeyValuePairOptional
	Instructions []*parser.Node
}

func (d *Dockerfile) Dump() string {
	result := make([]string, 0, len(d.Stages))
	for _, stage := range d.Stages {
		if dump := DumpAll(stage.Instructions); dump != "" {
			result = append(result, dump)
		}
	}
	return strings.Join(result, "\n")
}

func Parse(dockerfileContent string) (*Dockerfile, error) {
	result, err := parser.Parse(strings.NewReader(dockerfileContent))
	if err != nil {
		return nil, err
	}
	if len(result.AST.Children) == 0 {
		return nil, fmt.Errorf("received empty Dockerfile")
	}

	d := &Dockerfile{
		Raw:            dockerfileContent,
		Preamble:       &Preamble{},
		StagesByTarget: make(map[string]*Stage),
	}

	directiveParser := parser.DirectiveParser{}
	if directives, _ := directiveParser.ParseAll([]byte(dockerfileContent)); len(directives) > 0 {
		d.Directives = directives
		for _, directive := range directives {
			if strings.EqualFold(directive.Name, "syntax") {
				d.Syntax = directive.Value
				break
			}
		}
	}

	// Parse instructions with single loop
	isPreamble := true
	for _, instruction := range result.AST.Children {
		cmd := strings.ToLower(instruction.Value)

		if isPreamble && cmd == command.From {
			isPreamble = false
			d.Stages = append(d.Stages, parseStage(instruction))
			continue
		}

		if isPreamble {
			d.Preamble.Instructions = append(d.Preamble.Instructions, instruction)
			switch cmd {
			case command.Env:
				d.Preamble.Envs = append(d.Preamble.Envs, parseEnv(instruction)...)
			case command.Arg:
				d.Preamble.Args = append(d.Preamble.Args, parseArg(instruction))
			}
			continue
		}

		if cmd == command.From {
			d.Stages = append(d.Stages, parseStage(instruction))
			continue
		}

		lastStage := d.Stages[len(d.Stages)-1]
		lastStage.Instructions = append(lastStage.Instructions, instruction)
		switch cmd {
		case command.Env:
			lastStage.Envs = append(lastStage.Envs, parseEnv(instruction)...)
		case command.Arg:
			lastStage.Args = append(lastStage.Args, parseArg(instruction))
		case command.User:
			lastStage.Users = append(lastStage.Users, parseUser(instruction))
		}
	}

	for _, stage := range d.Stages {
		if stage.Target != "" {
			d.StagesByTarget[stage.Target] = stage
		}
	}

	return d, nil
}

func parseUser(instruction *parser.Node) instructions.KeyValuePair {
	value := instruction.Next.Value
	if strings.Contains(value, ":") && !strings.HasPrefix(value, "${") {
		value = strings.Split(value, ":")[0]
	}
	return instructions.KeyValuePair{Key: value}
}

func parseArg(instruction *parser.Node) instructions.KeyValuePairOptional {
	node := instruction.Next
	if node.Next != nil {
		value := node.Next.Value
		return instructions.KeyValuePairOptional{Key: node.Value, Value: &value}
	}
	if strings.Contains(node.Value, "=") {
		parts := strings.SplitN(node.Value, "=", 2)
		return instructions.KeyValuePairOptional{Key: parts[0], Value: &parts[1]}
	}
	return instructions.KeyValuePairOptional{Key: node.Value}
}

func parseEnv(instruction *parser.Node) []instructions.KeyValuePair {
	envs := make([]instructions.KeyValuePair, 0, 2) // Most ENV instructions have 1-2 pairs
	for node := instruction.Next; node != nil && node.Next != nil; node = node.Next.Next {
		envs = append(envs, instructions.KeyValuePair{
			Key:   strings.TrimSpace(node.Value),
			Value: strings.Trim(strings.ReplaceAll(node.Next.Value, "\\", ""), "\"'"),
		})
	}
	return envs
}

func parseStage(instruction *parser.Node) *Stage {
	var image, target string
	if next := instruction.Next; next != nil {
		image = next.Value
		if next.Next != nil && strings.EqualFold(next.Next.Value, "as") &&
			next.Next.Next != nil && next.Next.Next.Value != "" {
			target = next.Next.Next.Value
		}
	}
	return &Stage{
		BaseStage: BaseStage{
			Image:        image,
			Target:       target,
			Instructions: []*parser.Node{instruction},
		},
	}
}

func DumpAll(nodes []*parser.Node) string {
	if len(nodes) == 0 {
		return ""
	}
	children := make([]string, len(nodes))
	for i, n := range nodes {
		children[i] = DumpNode(n)
	}
	return strings.Join(children, "\n")
}

func DumpNode(node *parser.Node) string {
	var out strings.Builder
	if len(node.PrevComment) > 0 {
		out.WriteString("# ")
		out.WriteString(strings.Join(node.PrevComment, "\n# "))
		if node.Value != "" {
			out.WriteByte('\n')
		}
	}

	if node.Value != "" {
		out.WriteString(node.Value)
	}
	for _, child := range node.Flags {
		out.WriteByte(' ')
		out.WriteString(child)
	}
	if node.Next != nil {
		out.WriteByte(' ')
		out.WriteString(DumpNode(node.Next))
	}

	return out.String()
}
