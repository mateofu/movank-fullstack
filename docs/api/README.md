# Contrato HTTP

El [contrato OpenAPI](openapi.yaml) describe `/healthz` y `/readyz`.
Se ampliará al implementar autenticación, esquemas, errores, idempotencia y SSE
para estos endpoints pendientes:

- `POST /v1/products`
- `POST /v1/sales`
- `POST /v1/sales/{id}/pay`
- `GET /v1/sales/{id}`
- `GET /v1/dashboard/today`
- `GET /v1/dashboard/stream`
