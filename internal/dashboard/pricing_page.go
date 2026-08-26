package dashboard

func RenderPricing(data Data) ([]byte, error) {
	return renderPage(pricingTemplate, pricingScript, data)
}
