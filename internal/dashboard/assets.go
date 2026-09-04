package dashboard

import _ "embed"

var (
	//go:embed assets/styles.css
	styles []byte

	//go:embed assets/auth.js
	managementAuthScript []byte

	//go:embed assets/billing.html
	billingTemplate []byte

	//go:embed assets/billing.js
	billingScript []byte

	//go:embed assets/pricing.html
	pricingTemplate []byte

	//go:embed assets/pricing.js
	pricingScript []byte

	//go:embed assets/balances.html
	balancesTemplate []byte

	//go:embed assets/balances.js
	balancesScript []byte
)
