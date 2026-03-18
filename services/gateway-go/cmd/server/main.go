package main

import gatewayhttp "trustops-ecommerce-riskops-platform/services/gateway-go/internal/http"

func main() {
	app := gatewayhttp.NewRouter()
	app.Spin()
}
