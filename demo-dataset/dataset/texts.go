package dataset

// Summaries of background tasks and bodies of comments, in the language of the demo projects. A summary is drawn
// once per task type before the list repeats, so tasks read like the work of a team and not like their variant.
var (
	_titles = map[string][]string{
		"Story": {
			"Show delivery cost on the checkout page", "Let customers save a draft order", "Export orders to CSV",
			"Apply promo codes at checkout", "Send an email when an order ships", "Add order search by customer",
			"Show discount breakdown on the invoice", "Support partial refunds", "Paginate the order history",
			"Add a monthly revenue report", "Let support cancel an order", "Show estimated delivery date",
			"Add tiered discounts for wholesale", "Keep the cart between sessions", "Add order notes for couriers",
			"Group orders by warehouse in reports", "Allow changing the delivery address", "Add a weekly sales digest",
			"Show the order status timeline", "Import price lists from suppliers", "Add a returns request form",
			"Notify customers about back-in-stock items", "Add VAT to the order summary", "Rate-limit the public API",
			"Add audit log for price changes", "Support gift cards at checkout", "Show top products in the dashboard",
			"Add bulk status update for orders", "Let partners fetch invoices by API", "Add a retry button for failed payments",
			"Cache the product catalog", "Add a health page for the service", "Split the report by region",
			"Show payment method on the order card", "Add idempotency keys to order creation", "Archive orders older than two years",
		},
		"Bug": {
			"Discount rounds down instead of half up", "Report titles break on unicode slugs", "Order total ignores shipping",
			"Duplicate email on order confirmation", "Checkout fails for empty promo code", "Invoice PDF cuts the last line",
			"Search returns cancelled orders", "Wrong currency symbol for KZT", "Timeout when exporting large reports",
			"Refund button visible to viewers", "Delivery date shown in UTC", "Pagination skips the last page",
			"Webhook retried after success", "Cart loses items after login", "Revenue report double counts refunds",
			"Address form accepts empty city", "Status badge stuck on processing", "Price list import drops last row",
			"API returns 500 on unknown order", "Session expires during checkout", "Totals differ between list and card",
			"Health check reports ready before migrations",
		},
		"Task": {
			"Upgrade the HTTP router", "Move configuration to environment variables", "Add CI cache for Go modules",
			"Rotate the partner API key", "Clean up feature flags", "Document the release process",
			"Set up alerts for checkout errors", "Remove the legacy quotes endpoint", "Add load test for order creation",
			"Bump Go to the current release", "Split the reports package", "Write the on-call runbook",
			"Archive the old staging database", "Review access to the payroll export", "Add a dashboard for queue lag",
		},
	}
	_commentBodies = []string{
		"Picked this up.", "Pushed a first version, review is welcome.", "Reproduced it on staging.",
		"Waiting for the contract from Payments.", "Added tests for the edge cases.", "Fixed the review comments.",
		"Can we keep the old behaviour behind a flag?", "Checked with support, the case is real.", "Need a decision on the format.",
		"Deployed to staging, please verify.", "The root cause is in the rounding helper.", "Split the rest into a separate task.",
		"Looks good to me.", "Blocked until the migration is merged.", "Moved the discussion to the design doc.",
		"Verified on production.", "Updated the API docs as well.", "This also affects the monthly report.",
	}
)

// title is the next summary of the type: every summary of the list once, in an order drawn from the seed.
func (g *taskGen) title(typ string) string {
	list := _titles[typ]
	if g.titleOrder == nil {
		g.titleOrder, g.titleNext = map[string][]int{}, map[string]int{}
	}
	if g.titleOrder[typ] == nil {
		g.titleOrder[typ] = g.rng.Perm(len(list))
	}
	n := g.titleNext[typ]
	g.titleNext[typ]++
	return list[g.titleOrder[typ][n%len(list)]]
}
