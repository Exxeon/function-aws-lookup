package lookup

import (
	"regexp"
	"slices"
	"strings"
	"sync"
)

//go:generate go run ./gen

// arnFormat is an ARN format from the Service Authorization Reference, for
// example arn:${Partition}:rds:${Region}:${Account}:db:${DbInstanceName}, and
// the CloudFormation types whose resources have ARNs of that format.
type arnFormat struct {
	Format             string
	CloudFormationType []string
}

type compiledFormat struct {
	re      *regexp.Regexp
	literal int
	types   []string
}

var (
	formatsOnce      sync.Once                   //nolint:gochecknoglobals // Built once from the generated table.
	formatsByService map[string][]compiledFormat //nolint:gochecknoglobals // Built once from the generated table.
)

// CloudFormationTypes returns the CloudFormation types that a resource with
// this ARN can have, best match first. It returns nothing if the ARN matches
// no known format.
func CloudFormationTypes(arn string) []string {
	formatsOnce.Do(compileFormats)

	parts := strings.SplitN(arn, ":", 4)
	if len(parts) < 4 {
		return nil
	}

	var matches []compiledFormat
	for _, f := range formatsByService[parts[2]] {
		if f.re.MatchString(arn) {
			matches = append(matches, f)
		}
	}
	// The format with the most literal text is the most specific, for example
	// loadbalancer/app/${Name}/${Id} rather than loadbalancer/${Name}.
	slices.SortStableFunc(matches, func(a, b compiledFormat) int { return b.literal - a.literal })

	var types []string
	for _, m := range matches {
		for _, t := range m.types {
			if !slices.Contains(types, t) {
				types = append(types, t)
			}
		}
	}
	return types
}

var placeholder = regexp.MustCompile(`\$\{[^}]*\}|\*`)

func compileFormats() {
	formatsByService = map[string][]compiledFormat{}
	for _, f := range arnFormats {
		parts := strings.SplitN(f.Format, ":", 4)
		if len(parts) < 4 {
			continue
		}
		re, literal := formatRegexp(f.Format)
		formatsByService[parts[2]] = append(formatsByService[parts[2]], compiledFormat{re: re, literal: literal, types: f.CloudFormationType})
	}
}

// formatRegexp turns an ARN format into a regular expression. The last
// placeholder matches the rest of the ARN, because names such as log group
// and ECR repository names can contain slashes.
func formatRegexp(format string) (*regexp.Regexp, int) {
	locs := placeholder.FindAllStringIndex(format, -1)

	var b strings.Builder
	b.WriteString("^")
	literal, last := 0, 0
	for i, loc := range locs {
		b.WriteString(regexp.QuoteMeta(format[last:loc[0]]))
		literal += loc[0] - last
		switch p := format[loc[0]:loc[1]]; {
		case p == "${Partition}":
			b.WriteString(`[^:]+`)
		case p == "${Region}", p == "${Account}":
			b.WriteString(`[^:]*`)
		case i == len(locs)-1 || p == "*":
			b.WriteString(`.+`)
		default:
			b.WriteString(`[^:/]+`)
		}
		last = loc[1]
	}
	b.WriteString(regexp.QuoteMeta(format[last:]))
	literal += len(format) - last
	b.WriteString("$")

	return regexp.MustCompile(b.String()), literal
}
