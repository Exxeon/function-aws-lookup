package lookup_test

import (
	"github.com/Exxeon/function-aws-lookup/internal/lookup"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Field", func() {
	const props = `{
		"DBInstanceIdentifier": "orders",
		"DbiResourceId": "db-ABC123",
		"Port": 5432,
		"Endpoint": {"Address": "orders.abc.eu-west-1.rds.amazonaws.com", "Port": "5432"},
		"Tags": [{"Key": "team", "Value": "payments"}],
		"a/b": {"c~d": "escaped"},
		"Empty": "",
		"Enabled": true
	}`

	DescribeTable("returns the value of a field",
		func(field, want string) {
			Expect(lookup.Field(props, field)).To(Equal(want))
		},
		Entry("top-level property", "DbiResourceId", "db-ABC123"),
		Entry("nested property", "Endpoint/Address", "orders.abc.eu-west-1.rds.amazonaws.com"),
		Entry("leading slash", "/Endpoint/Address", "orders.abc.eu-west-1.rds.amazonaws.com"),
		Entry("array index", "Tags/0/Value", "payments"),
		Entry("number", "Port", "5432"),
		Entry("JSON Pointer escaping", "a~1b/c~0d", "escaped"),
	)

	DescribeTable("returns an error for an unusable field",
		func(field, msg string) {
			_, err := lookup.Field(props, field)
			Expect(err).To(MatchError(ContainSubstring(msg)))
		},
		Entry("missing property", "ResourceId", `resource has no property "ResourceId", available properties are DBInstanceIdentifier, DbiResourceId, Empty, Enabled, Endpoint, Port, Tags, a/b`),
		Entry("missing nested property", "Endpoint/Host", `available properties of Endpoint are Address, Port`),
		Entry("invalid index", "Tags/3", "is a list of 1 items"),
		Entry("path through a string", "DbiResourceId/x", `property DbiResourceId has no property "x"`),
		Entry("object", "Endpoint", "is not a string or number"),
		Entry("boolean", "Enabled", "is not a string or number"),
		Entry("empty value", "Empty", "is empty"),
		Entry("empty segment", "Endpoint//Address", "empty path segments"),
	)

	It("returns an error for invalid JSON", func() {
		_, err := lookup.Field("{", "a")
		Expect(err).To(MatchError(ContainSubstring("cannot parse resource properties")))
	})
})
