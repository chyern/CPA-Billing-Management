package dashboard

func RenderBilling(data Data) ([]byte, error) {
	return renderPage(billingTemplate, billingScript, data)
}
