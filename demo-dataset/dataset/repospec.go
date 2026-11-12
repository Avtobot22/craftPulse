package dataset

// Fixed description of the four fixture repositories. Commit times, authors and messages define the SHAs;
// file contents come from fixtures/<key>/base, documents are rendered from the tables below.

type repoSpec struct {
	key            string
	organizationID string
	instanceID     string
	areaID         string
	path           string
	restricted     bool
	docRoots       []string
	docs           []docSpec
	specs          []openAPISpec
	commits        []commitPlan
	defects        []defectSpec
	brokenLinks    []BrokenLink
	steps          []stepSpec
}

// commitPlan takes the files whose path starts with one of prefixes; a file goes to the commit with its
// longest matching prefix.
type commitPlan struct {
	at       string
	authorID string
	message  string
	prefixes []string
}

// defectSpec finds the defect line by the trimmed line prefix in the head tree.
type defectSpec struct {
	id, kind, tool, rule, file, linePrefix, exception string
	fixedByStep                                       int
}

type textEdit struct {
	file, old, new string
}

// stepSpec replaces files with fixtures/<key>/step-<N> and applies the edits to rendered documents.
type stepSpec struct {
	step  int
	edits []textEdit
	fixes []string
}

func repoSpecs() []repoSpec {
	return []repoSpec{ordersRepo(), billingRepo(), restrictedRepo(), privateRepo()}
}

func ordersRepo() repoSpec {
	const lead, dev5, dev6, dev7, dev8 = "demo-person-03", "demo-person-05", "demo-person-06", "demo-person-07", "demo-person-08"
	return repoSpec{
		key: "orders-service", organizationID: "org-1", instanceID: "gitlab-1", areaID: "repo-1", path: "demo/orders-service",
		docRoots: []string{"docs"},
		docs: []docSpec{
			{path: "docs/README.md", title: "Orders service documentation", owner: lead, index: true},
			{path: "docs/architecture.md", title: "Architecture", owner: lead, sections: []string{"Components", "Data flow"}, links: []string{"api/overview.md", "adr/0001-http-router.md"}},
			{path: "docs/getting-started.md", title: "Getting started", owner: dev5, sections: []string{"Prerequisites", "Run locally"}, links: []string{"configuration.md#environment", "../README.md"}},
			{path: "docs/configuration.md", title: "Configuration", owner: lead, sections: []string{"Environment", "Defaults"}, links: []string{"adr/0003-config-from-env.md"}},
			{path: "docs/glossary.md", title: "Glossary", sections: []string{"Kopeck", "Quote"}},
			{path: "docs/changelog.md", title: "Changelog", sections: []string{"March 2026", "June 2026"}, links: []string{"api/quotes.md"}},
			{path: "docs/api/overview.md", title: "API overview", owner: dev7, sections: []string{"Authentication", "Limits"}, links: []string{"quotes.md", "reports.md", "errors.md#rate-limits", "orders.openapi.yaml"}},
			{path: "docs/api/errors.md", title: "Errors", owner: dev7, sections: []string{"Error format", "Rate limiting", "Validation"}, links: []string{"overview.md"}},
			{path: "docs/api/quotes.md", title: "Quotes", owner: dev5, sections: []string{"Request", "Response"}, links: []string{"errors.md#error-format", "../glossary.md#quote"}},
			{path: "docs/api/reports.md", title: "Reports", owner: dev6, sections: []string{"Titles"}, links: []string{"reports.openapi.yaml", "../adr/0004-report-titles.md"}},
			{path: "docs/runbooks/deploy.md", title: "Deploy", owner: dev8, sections: []string{"Steps", "Rollback"}, links: []string{"../ops/rollback.md", "incident.md#escalation"}},
			{path: "docs/runbooks/incident.md", title: "Incident response", owner: dev8, sections: []string{"Triage", "Escalation"}, links: []string{"on-call.md"}},
			{path: "docs/runbooks/on-call.md", title: "On-call", owner: dev8, sections: []string{"Rotation"}, links: []string{"incident.md"}},
			{path: "docs/runbooks/database.md", title: "Database maintenance", sections: []string{"Backups"}},
			{path: "docs/adr/0001-http-router.md", title: "ADR 0001: HTTP router", owner: lead, sections: []string{"Context", "Decision"}},
			{path: "docs/adr/0002-money-in-kopecks.md", title: "ADR 0002: Money in kopecks", owner: lead, sections: []string{"Context", "Decision"}, links: []string{"../glossary.md#kopeck"}},
			{path: "docs/adr/0003-config-from-env.md", title: "ADR 0003: Configuration from environment", owner: lead, sections: []string{"Context", "Decision"}, links: []string{"../configuration.md"}},
			{path: "docs/adr/0004-report-titles.md", title: "ADR 0004: Report titles", sections: []string{"Context", "Decision"}},
			{path: "docs/process/code-review.md", title: "Code review", owner: lead, sections: []string{"Checklist"}, links: []string{"testing.md"}},
			{path: "docs/process/release.md", title: "Release", owner: dev6, sections: []string{"Versioning"}, links: []string{"../changelog.md", "../runbooks/deploy.md#steps"}},
			{path: "docs/process/testing.md", title: "Testing", sections: []string{"Unit tests"}},
			{path: "docs/process/security.md", title: "Security", owner: lead, sections: []string{"Secrets"}, links: []string{"../configuration.md#defaults"}},
		},
		specs: []openAPISpec{
			{path: "docs/api/orders.openapi.yaml", title: "Orders API", valid: true, operations: []specOperation{{"post", "/v1/quotes", "createQuote", "Quote an order", "Quote with the total in kopecks"}}},
			{path: "docs/api/reports.openapi.yaml", title: "Reports API", valid: true, operations: []specOperation{{"get", "/v1/reports/{name}/title", "getReportTitle", "Render a report title", "Report title as plain text"}}},
			{path: "docs/api/legacy-quotes.openapi.yaml", title: "Legacy quotes API", operations: []specOperation{{"post", "/v0/quotes", "createLegacyQuote", "Quote an order with the legacy format", ""}}},
		},
		commits: []commitPlan{
			{"2026-01-12T07:10:00Z", "org-1/person-03", "Initialize orders service", []string{"go.mod", "README.md"}},
			{"2026-01-20T09:30:00Z", "org-1/person-05", "Add order totals with percentage discounts", []string{"internal/order/"}},
			{"2026-02-17T11:05:00Z", "org-1/person-03", "Read configuration from the environment", []string{"internal/config/"}},
			{"2026-03-04T08:45:00Z", "org-1/person-06", "Add report titles", []string{"internal/report/"}},
			{"2026-03-18T13:20:00Z", "org-1/person-07", "Expose quote and report endpoints", []string{"cmd/", "internal/httpapi/"}},
			{"2026-04-01T10:00:00Z", "org-1/person-03", "Document architecture and local setup", []string{"docs/README.md", "docs/architecture.md", "docs/getting-started.md", "docs/configuration.md"}},
			{"2026-04-22T12:40:00Z", "org-1/person-07", "Describe the public API", []string{"docs/api/"}},
			{"2026-05-13T09:15:00Z", "org-1/person-08", "Add deployment and incident runbooks", []string{"docs/runbooks/"}},
			{"2026-06-10T14:30:00Z", "org-1/person-03", "Record architecture decisions", []string{"docs/adr/"}},
			{"2026-07-08T07:50:00Z", "org-1/person-09", "Write down team processes", []string{"docs/process/"}},
			{"2026-09-16T10:25:00Z", "org-1/person-06", "Add glossary and changelog", []string{"docs/glossary.md", "docs/changelog.md"}},
		},
		defects: []defectSpec{
			{id: "orders-test-rounding", kind: DefectTest, tool: "go test", rule: "demo.invalid/orders/internal/order.TestApplyDiscountRoundsHalfUp", file: "internal/order/discount_test.go", linePrefix: `t.Fatalf("ApplyDiscount(1050, 5)`},
			{id: "orders-lint-errcheck", kind: DefectLint, tool: "golangci-lint", rule: "errcheck", file: "internal/httpapi/handler.go", linePrefix: "json.NewEncoder(w).Encode(quote{", fixedByStep: 7},
			{id: "orders-lint-exception", kind: DefectLint, tool: "golangci-lint", rule: "staticcheck/SA1019", file: "internal/report/report.go", linePrefix: "return strings.Title(words)", exception: "//nolint:staticcheck"},
			{id: "orders-secret", kind: DefectSecurity, tool: "gitleaks", rule: "generic-api-key", file: "internal/config/config.go", linePrefix: "const defaultPartnerAPIKey = "},
		},
		brokenLinks: []BrokenLink{
			{ID: "orders-doc-broken-file", Document: "docs/runbooks/deploy.md", Target: "../ops/rollback.md", Kind: BrokenFile},
			{ID: "orders-doc-broken-anchor", Document: "docs/api/overview.md", Target: "errors.md#rate-limits", Kind: BrokenAnchor},
		},
		steps: []stepSpec{
			{step: 5},
			{step: 7, edits: []textEdit{{"docs/api/overview.md", "(errors.md#rate-limits)", "(errors.md#rate-limiting)"}}, fixes: []string{"orders-lint-errcheck", "orders-doc-broken-anchor"}},
		},
	}
}

func billingRepo() repoSpec {
	const p10, p11, p12, p13, p14 = "demo-person-10", "demo-person-11", "demo-person-12", "demo-person-13", "demo-person-14"
	return repoSpec{
		key: "billing-api", organizationID: "org-1", instanceID: "github-1", areaID: "repo-2", path: "demo/billing-api",
		restricted: true, docRoots: []string{"docs"},
		docs: []docSpec{
			{path: "docs/README.md", title: "Billing API documentation", owner: p10, index: true},
			{path: "docs/overview.md", title: "Overview", owner: p10, sections: []string{"Scope", "Components"}, links: []string{"pricing/tiers.md", "api/invoices.md"}},
			{path: "docs/changelog.md", title: "Changelog", sections: []string{"February 2026", "August 2026"}},
			{path: "docs/faq.md", title: "FAQ", owner: p11, sections: []string{"Questions"}, links: []string{"pricing/rounding.md#half-up"}},
			{path: "docs/pricing/tiers.md", title: "Tiers", owner: p11, sections: []string{"Tiers", "Examples"}, links: []string{"rounding.md"}},
			{path: "docs/pricing/currencies.md", title: "Currencies", owner: p11, sections: []string{"Supported currencies"}},
			{path: "docs/pricing/rounding.md", title: "Rounding", owner: p11, sections: []string{"Half-up", "Examples"}},
			{path: "docs/pricing/discounts.md", title: "Discounts", owner: p12, sections: []string{"Rules"}, links: []string{"tiers.md#examples"}},
			{path: "docs/pricing/taxes.md", title: "Taxes", sections: []string{"VAT"}},
			{path: "docs/api/invoices.md", title: "Invoices", owner: p12, sections: []string{"Create", "Read"}, links: []string{"billing.openapi.yaml", "../pricing/tiers.md#tiers"}},
			{path: "docs/api/payments.md", title: "Payments", owner: p12, sections: []string{"Statuses"}},
			{path: "docs/api/webhooks.md", title: "Webhooks", owner: p13, sections: []string{"Delivery", "Retries"}, links: []string{"webhooks.openapi.yaml"}},
			{path: "docs/guides/onboarding.md", title: "Onboarding", owner: p13, sections: []string{"First day"}, links: []string{"local-setup.md"}},
			{path: "docs/guides/local-setup.md", title: "Local setup", owner: p13, sections: []string{"Requirements"}, links: []string{"../../README.md"}},
			{path: "docs/guides/testing.md", title: "Testing", owner: p14, sections: []string{"Unit tests"}},
			{path: "docs/guides/troubleshooting.md", title: "Troubleshooting", sections: []string{"Common errors"}},
			{path: "docs/adr/0001-tiered-pricing.md", title: "ADR 0001: Tiered pricing", owner: p10, sections: []string{"Context", "Decision"}},
			{path: "docs/adr/0002-amounts-in-kopecks.md", title: "ADR 0002: Amounts in kopecks", owner: p10, sections: []string{"Context", "Decision"}},
			{path: "docs/adr/0003-webhook-retries.md", title: "ADR 0003: Webhook retries", owner: p10, sections: []string{"Context", "Decision"}, links: []string{"../api/webhooks.md#retries"}},
			{path: "docs/adr/0004-currency-list.md", title: "ADR 0004: Currency list", owner: p10, sections: []string{"Context", "Decision"}},
			{path: "docs/process/release.md", title: "Release", owner: p14, sections: []string{"Versioning"}, links: []string{"../changelog.md"}},
			{path: "docs/process/support.md", title: "Support", owner: p14, sections: []string{"Requests"}},
			{path: "docs/process/security.md", title: "Security", owner: p10, sections: []string{"Access"}},
		},
		specs: []openAPISpec{
			{path: "docs/api/billing.openapi.yaml", title: "Billing API", valid: true, operations: []specOperation{{"post", "/v1/invoices", "createInvoice", "Create an invoice", "Created invoice"}, {"get", "/v1/invoices/{id}", "getInvoice", "Read an invoice", "Invoice"}}},
			{path: "docs/api/webhooks.openapi.yaml", title: "Billing webhooks", valid: true, operations: []specOperation{{"post", "/v1/webhooks", "registerWebhook", "Register a webhook", "Registered webhook"}}},
		},
		commits: []commitPlan{
			{"2026-01-26T08:00:00Z", "org-1/person-10", "Initialize billing API", []string{"go.mod", "README.md"}},
			{"2026-02-09T10:30:00Z", "org-1/person-11", "Add tiered pricing", []string{"pricing/"}},
			{"2026-03-23T09:00:00Z", "org-1/person-10", "Document pricing rules", []string{"docs/README.md", "docs/overview.md", "docs/pricing/"}},
			{"2026-05-04T12:15:00Z", "org-1/person-12", "Describe invoice, payment and webhook API", []string{"docs/api/"}},
			{"2026-06-15T08:40:00Z", "org-1/person-13", "Add developer guides", []string{"docs/guides/"}},
			{"2026-07-20T11:10:00Z", "org-1/person-10", "Record architecture decisions", []string{"docs/adr/"}},
			{"2026-08-31T09:45:00Z", "org-1/person-14", "Describe release and support process", []string{"docs/process/"}},
			{"2026-10-05T13:00:00Z", "org-1/person-11", "Add FAQ and changelog", []string{"docs/faq.md", "docs/changelog.md"}},
		},
	}
}

func restrictedRepo() repoSpec {
	return repoSpec{
		key: "payroll-export", organizationID: "org-1", instanceID: "gitlab-1", areaID: "restricted-repo", path: "demo/payroll-export",
		restricted: true, docRoots: []string{},
		commits: []commitPlan{
			{"2026-02-02T09:00:00Z", "org-1/person-03", "Initialize payroll export", []string{"go.mod", "README.md"}},
			{"2026-02-16T12:30:00Z", "org-1/person-11", "Write payroll rows as CSV", []string{"export/"}},
		},
	}
}

func privateRepo() repoSpec {
	return repoSpec{
		key: "private-service", organizationID: "org-2", instanceID: "gitlab-1", areaID: "private-repo", path: "other/private-service",
		restricted: true, docRoots: []string{"docs"},
		docs: []docSpec{{path: "docs/overview.md", title: "Private service", owner: "demo-other-person", sections: []string{"Purpose"}}},
		commits: []commitPlan{
			{"2026-03-02T08:20:00Z", "org-2/person-01", "Initialize private service", []string{"go.mod", "status/"}},
			{"2026-03-10T10:05:00Z", "org-2/person-01", "Describe the service", []string{"docs/"}},
		},
	}
}
