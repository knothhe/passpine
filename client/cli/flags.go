package cli

import "strings"

const (
	noValue = iota
	optionalValue
	requiredValue
)

type flagSpec struct {
	short string
	long  string
	value int
}

// parseFlags follows the getopt conventions pass relies on: permuted
// arguments, combined short flags, `--`, and optional values attached as
// `-c2` or `--clip=2`. Results are keyed by the long name.
func (c *command) parseFlags(args []string, specs ...flagSpec) (map[string]string, []string, error) {
	flags, positional := map[string]string{}, []string{}
	find := func(match func(flagSpec) bool) (flagSpec, bool) {
		for _, s := range specs {
			if match(s) {
				return s, true
			}
		}
		return flagSpec{}, false
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--":
			return flags, append(positional, args[i+1:]...), nil
		case strings.HasPrefix(arg, "--"):
			name, value, hasValue := strings.Cut(arg[2:], "=")
			spec, ok := find(func(s flagSpec) bool { return s.long == name })
			if !ok || (hasValue && spec.value == noValue) {
				return nil, nil, c.fail("无效的选项：%s", arg)
			}
			if spec.value == requiredValue && !hasValue {
				if i+1 >= len(args) {
					return nil, nil, c.fail("选项 %s 需要一个值", arg)
				}
				i++
				value = args[i]
			}
			flags[spec.long] = value
		case strings.HasPrefix(arg, "-") && arg != "-":
			for j := 1; j < len(arg); j++ {
				spec, ok := find(func(s flagSpec) bool { return s.short == arg[j:j+1] })
				if !ok {
					return nil, nil, c.fail("无效的选项：%s", "-"+arg[j:j+1])
				}
				if spec.value == noValue {
					flags[spec.long] = ""
					continue
				}
				value := arg[j+1:]
				if value == "" && spec.value == requiredValue {
					if i+1 >= len(args) {
						return nil, nil, c.fail("选项 %s 需要一个值", "-"+spec.short)
					}
					i++
					value = args[i]
				}
				flags[spec.long] = value
				break
			}
		default:
			positional = append(positional, arg)
		}
	}
	return flags, positional, nil
}
