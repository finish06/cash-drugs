package handler

import (
	"net/http"

	"github.com/finish06/cash-drugs/docs"
)

// ServeOpenAPISpec serves the generated OpenAPI spec as JSON.
//
// @Summary      OpenAPI specification
// @Description  Returns the OpenAPI 3.0 JSON specification for this API.
// @Tags         system
// @Produce      json
// @Success      200  {object}  object
// @Router       /openapi.json [get]
func ServeOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(docs.SwaggerInfo.ReadDoc()))
}
