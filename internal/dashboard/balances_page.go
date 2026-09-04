package dashboard

func RenderBalances(data Data) ([]byte, error) {
	return renderPage(balancesTemplate, balancesScript, data)
}
