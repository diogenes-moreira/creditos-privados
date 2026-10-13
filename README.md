# Prestia

Sistema de gestión de microcréditos para Argentina. Clientes obtienen cuentas corrientes, líneas de crédito y pueden solicitar préstamos con amortización francesa o alemana. Administradores aprueban créditos, ajustan pagos y gestionan el portafolio con dashboard de KPIs. Comercios adheridos registran ventas a crédito y consultan su saldo.

## Arquitectura

```
creditos/
├── backend/          # Go (Gin + GORM) - API REST + Swagger
├── frontend/         # React (MUI + TypeScript) - SPA
├── static-site/      # Astro - Landing page
└── docker-compose.yml
```

| Componente | Tecnología | Puerto |
|---|---|---|
| Backend API | Go 1.22, Gin, GORM | `:8080` |
| Frontend | React 18, MUI 5, Vite | `:3000` |
| Landing Page | Astro 4.5 | `:4321` |
| Base de datos | PostgreSQL 16 | `:5432` |
| Swagger UI | swaggo/gin-swagger | `:8080/swagger/index.html` |

## Requisitos previos

- [Docker](https://docs.docker.com/get-docker/) y Docker Compose
- [Go 1.22+](https://go.dev/dl/) (para desarrollo local)
- [Node.js 20+](https://nodejs.org/) (para desarrollo local)
- [Make](https://www.gnu.org/software/make/)

## Inicio rápido con Docker

### 1. Clonar el repositorio

```bash
git clone git@github.com:diogenes-moreira/creditos.git
cd creditos
```

### 2. Configurar variables de entorno

```bash
cp .env.example .env
```

Editar `.env` si es necesario. Los valores por defecto funcionan para desarrollo local.

### 3. Levantar todos los servicios

```bash
make docker-up
```

Esto construye y levanta:
- **PostgreSQL** en `localhost:5432`
- **Backend API** en `localhost:8080`
- **Frontend** en `localhost:3000`

### 4. Cargar datos de prueba

```bash
# Opción 1: con Docker
docker exec -it creditos-backend ./seed

# Opción 2: local (requiere Go y PostgreSQL corriendo)
make seed
```

El seed genera:
- 2 administradores (`admin@prestia.com.ar` / `admin123`, `supervisor@prestia.com.ar` / `admin123`)
- 50 clientes con datos argentinos (DNI, CUIT válidos)
- 30 líneas de crédito en distintos estados
- 40 préstamos con cuotas y pagos
- Registros de auditoría

### 5. Acceder

| Servicio | URL |
|---|---|
| Frontend | http://localhost:3000 |
| API | http://localhost:8080/api/v1 |
| Swagger UI | http://localhost:8080/swagger/index.html |
| Health Check | http://localhost:8080/health |

## Desarrollo local

### Backend

```bash
# Instalar dependencias
cd backend && go mod download

# Copiar y configurar variables
cp ../.env.example ../.env

# Ejecutar servidor (requiere PostgreSQL corriendo)
make run

# Ejecutar tests
make test

# Solo tests de dominio
make test-domain

# Lint
make lint

# Generar documentación Swagger (requiere swag CLI)
# go install github.com/swaggo/swag/cmd/swag@latest
cd backend && swag init -g cmd/server/main.go
```

### Frontend

```bash
# Instalar dependencias
make frontend-install

# Servidor de desarrollo (hot reload)
make frontend-dev

# Build de producción
make frontend-build
```

El frontend en desarrollo corre en `http://localhost:5173` y proxea `/api` al backend.

### Landing Page (Astro)

```bash
cd static-site
npm install
npm run dev    # http://localhost:4321
```

## Variables de entorno

| Variable | Descripción | Default |
|---|---|---|
| `DB_HOST` | Host de PostgreSQL | `localhost` |
| `DB_PORT` | Puerto de PostgreSQL | `5432` |
| `DB_USER` | Usuario de PostgreSQL | `creditos` |
| `DB_PASSWORD` | Contraseña de PostgreSQL | `creditos_secret` |
| `DB_NAME` | Nombre de la base de datos | `creditos` |
| `SERVER_PORT` | Puerto del backend | `8080` |
| `GIN_MODE` | Modo de Gin (`debug`/`release`) | `debug` |
| `JWT_SECRET` | Secreto para firmar tokens JWT | `change-me-in-production` |
| `JWT_EXPIRATION_HOURS` | Duración del token en horas | `24` |
| `FIREBASE_PROJECT_ID` | ID del proyecto Firebase (opcional en dev) | - |
| `FIREBASE_CREDENTIALS_FILE` | Credenciales de Firebase (opcional en dev) | - |
| `GCS_BUCKET` | Bucket de Google Cloud Storage (opcional en dev) | - |
| `GCS_CREDENTIALS_FILE` | Credenciales de GCS (opcional en dev) | - |
| `LOCAL_STORAGE_PATH` | Path de almacenamiento local (fallback) | `./storage` |
| `OTP_PROVIDER` | Gateway de WhatsApp: `kapso` o `twilio` (vacío = el que esté configurado) | - |
| `KAPSO_API_KEY` | Habilita el envío del OTP por WhatsApp vía Kapso | - |
| `KAPSO_PHONE_NUMBER_ID` | ID del número conectado en Kapso (no el número) | - |
| `KAPSO_TEMPLATE_NAME` | Plantilla aprobada con el código en `{{1}}` | - |
| `KAPSO_TEMPLATE_LANGUAGE` | Idioma de la plantilla | `es` |
| `TWILIO_ACCOUNT_SID` | Habilita el envío del OTP por WhatsApp vía Twilio | - |
| `TWILIO_AUTH_TOKEN` | Token de Twilio | - |
| `TWILIO_WHATSAPP_FROM` | Remitente WhatsApp de Twilio en E.164 | - |
| `TWILIO_WHATSAPP_TEMPLATE_SID` | Template aprobado (`HX...`) con el código en `{{1}}` | - |
| `ANTHROPIC_API_KEY` | Habilita el análisis de riesgo por prompt (opcional) | - |
| `RISK_ANALYSIS_MODEL` | Modelo usado para el análisis de riesgo | `claude-opus-5` |
| `RISK_ANALYSIS_EFFORT` | Profundidad del razonamiento (`low`..`max`) | `high` |
| `RISK_ANALYSIS_MAX_TOKENS` | Tope de tokens por análisis | `16000` |

## API Endpoints

### Públicos

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/health` | Health check |
| `GET` | `/health/ready` | Readiness probe |
| `POST` | `/api/v1/auth/register` | Registro de usuario |
| `POST` | `/api/v1/auth/login` | Login (retorna JWT) |

### Cliente (requiere JWT)

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/api/v1/me/profile` | Mi perfil |
| `PUT` | `/api/v1/me/profile` | Actualizar perfil |
| `PUT` | `/api/v1/me/mercadopago` | Configurar MercadoPago |
| `GET` | `/api/v1/me/account` | Mi cuenta corriente |
| `GET` | `/api/v1/me/account/movements` | Movimientos de cuenta |
| `POST` | `/api/v1/loans/simulate` | Simular préstamo |
| `POST` | `/api/v1/me/loans` | Solicitar préstamo |
| `GET` | `/api/v1/me/loans` | Mis préstamos |
| `GET` | `/api/v1/me/loans/:id` | Detalle de préstamo |
| `POST` | `/api/v1/me/loans/:loanId/payments` | Registrar pago |
| `GET` | `/api/v1/me/payments` | Mis pagos |
| `GET` | `/api/v1/me/risk-profile` | Mis datos financieros |
| `PUT` | `/api/v1/me/risk-profile` | Cargar/actualizar mis datos financieros |
| `GET` | `/api/v1/me/credit-availability` | Mi crédito disponible |
| `POST` | `/api/v1/risk/simulate` | Precalificación sin persistir |

### Administrador (requiere JWT + rol admin)

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/api/v1/admin/clients` | Listar clientes |
| `GET` | `/api/v1/admin/clients/:id` | Detalle de cliente |
| `GET` | `/api/v1/admin/clients/search` | Buscar clientes |
| `POST` | `/api/v1/admin/clients/:id/block` | Bloquear cliente |
| `POST` | `/api/v1/admin/clients/:id/unblock` | Desbloquear cliente |
| `POST` | `/api/v1/admin/credit-lines` | Crear línea de crédito |
| `GET` | `/api/v1/admin/credit-lines/pending` | Líneas pendientes |
| `POST` | `/api/v1/admin/credit-lines/:id/approve` | Aprobar línea |
| `POST` | `/api/v1/admin/credit-lines/:id/reject` | Rechazar línea |
| `GET` | `/api/v1/admin/loans/pending` | Préstamos pendientes |
| `POST` | `/api/v1/admin/loans/:id/approve` | Aprobar préstamo |
| `POST` | `/api/v1/admin/loans/:id/disburse` | Desembolsar préstamo |
| `POST` | `/api/v1/admin/loans/:id/cancel` | Cancelar préstamo |
| `POST` | `/api/v1/admin/loans/:id/prepay` | Cancelación anticipada |
| `PUT` | `/api/v1/admin/payments/:id/adjust` | Ajustar pago |
| `GET` | `/api/v1/admin/dashboard/portfolio` | Resumen portafolio |
| `GET` | `/api/v1/admin/dashboard/delinquency` | Métricas morosidad |
| `GET` | `/api/v1/admin/dashboard/kpis` | KPIs generales |
| `GET` | `/api/v1/admin/dashboard/trends/disbursements` | Tendencia desembolsos |
| `GET` | `/api/v1/admin/dashboard/trends/collections` | Tendencia cobranzas |
| `GET` | `/api/v1/admin/risk-profiles` | Listar datos de riesgo cargados |
| `PUT` | `/api/v1/admin/risk-profiles` | Cargar/actualizar datos de riesgo de un cliente |
| `GET` | `/api/v1/admin/clients/:id/risk-profile` | Datos de riesgo del cliente |
| `POST` | `/api/v1/admin/clients/:id/risk-assessments` | Evaluar riesgo del cliente |
| `GET` | `/api/v1/admin/clients/:id/risk-assessments` | Historial de evaluaciones |
| `POST` | `/api/v1/admin/clients/:id/risk-assessments/apply` | Aplicar límite recomendado a la línea |
| `GET` | `/api/v1/admin/clients/:id/credit-availability` | Disponibilidad de crédito |
| `GET` | `/api/v1/admin/risk-assessments/pending` | Evaluaciones en revisión manual |
| `GET` | `/api/v1/admin/risk-models` | Versiones del modelo de análisis |
| `POST` | `/api/v1/admin/risk-models` | Crear modelo de análisis |
| `GET` | `/api/v1/admin/risk-models/active` | Modelo activo (prompt + barandas) |
| `GET` | `/api/v1/admin/risk-models/analyzer` | Motor de análisis configurado |
| `POST` | `/api/v1/admin/risk-models/preview` | Previsualizar un prompt sin guardarlo |
| `PUT` | `/api/v1/admin/risk-models/:id` | Editar una versión |
| `POST` | `/api/v1/admin/risk-models/:id/versions` | Guardar una nueva versión |
| `POST` | `/api/v1/admin/risk-models/:id/activate` | Activar una versión |
| `GET` | `/api/v1/admin/audit` | Registros de auditoría |

Documentación interactiva completa en Swagger UI: `http://localhost:8080/swagger/index.html`

## Estructura del backend

```
backend/
├── cmd/
│   ├── server/main.go          # Entry point
│   └── seed/main.go            # Generador de datos de prueba
├── docs/                        # Swagger generado (swagger.json, swagger.yaml)
├── pkg/
│   ├── money/                   # Aritmética segura con shopspring/decimal
│   └── validator/               # Validación CUIT/DNI argentino
└── internal/
    ├── domain/
    │   ├── model/               # Entidades con lógica de negocio
    │   └── port/                # Interfaces (repositorios, servicios)
    ├── application/
    │   ├── service/             # Casos de uso
    │   └── dto/                 # Request/Response DTOs + mappers
    └── infrastructure/
        ├── config/              # Configuración por env vars
        ├── persistence/postgres/# Repositorios GORM
        ├── http/gin/            # Handlers, middleware, router
        ├── auth/                # Adaptador JWT (Firebase en producción)
        ├── storage/             # Almacenamiento local (GCS en producción)
        └── pdf/                 # Generación de PDFs
```

## Estructura del frontend

```
frontend/src/
├── api/                 # Cliente Axios, endpoints, tipos TypeScript
├── auth/                # Contexto de autenticación, rutas protegidas
├── components/          # AppLayout, DataTable, StatusBadge, KPICard, etc.
├── i18n/                # Traducciones ES/EN/PT con react-i18next
├── pages/
│   ├── auth/            # Login, Register
│   ├── client/          # Dashboard, Account, Loans, Payments, Profile
│   ├── admin/           # Dashboard, Clients, Credits, Loans, Payments, Audit, Vendors
│   └── vendor/          # Dashboard, NewPurchase, PurchaseHistory, Balance, Profile
└── theme.ts             # Tema MUI personalizado
```

## Comandos Make

```bash
make build             # Compilar binarios Go
make run               # Ejecutar backend localmente
make test              # Ejecutar todos los tests
make test-domain       # Tests solo del dominio
make seed              # Cargar datos de prueba
make docker-up         # Levantar con Docker Compose
make docker-down       # Detener servicios Docker
make docker-logs       # Ver logs de Docker
make lint              # Ejecutar go vet
make clean             # Limpiar binarios y volúmenes
make frontend-install  # Instalar deps del frontend
make frontend-dev      # Servidor de desarrollo frontend
make frontend-build    # Build de producción frontend
```

## Internacionalización (i18n)

El frontend soporta tres idiomas:
- Español (es) - por defecto
- English (en)
- Português (pt)

El idioma se detecta automáticamente del navegador. Se puede cambiar desde el selector de idioma en la barra superior.

## Riesgo crediticio

El módulo de riesgo responde una sola pregunta: **cuánto crédito se le puede dar a este solicitante**. Tiene tres piezas.

**1. Datos del solicitante.** Ingresos, empleo, gastos, obligaciones, personas a cargo, vivienda y datos de buró. Se cargan por tres vías, todas contra el mismo endpoint del dominio: el cliente desde su portal (`/me/risk-profile`), un operador desde la ficha del cliente, o una integración vía API. Los campos que son *atestación del prestador* — ingresos verificados, puntaje de buró, antecedentes negativos — no se pueden autodeclarar: en una carga hecha por el cliente o por un comercio se conservan los valores del registro y se ignora lo que venga en el request.

**2. Modelo de análisis, editable por el usuario.** Es una entidad versionada que combina:

- **Un prompt de análisis** que el administrador edita desde *Modelo de riesgo* en el panel. Los marcadores `{{client}}`, `{{profile}}`, `{{history}}`, `{{policy}}`, `{{baseline}}`, `{{currency}}`, `{{country}}` y `{{today}}` se reemplazan con los datos reales al evaluar. El editor valida los marcadores, previsualiza el prompt renderizado contra un cliente real antes de guardar, y cada edición crea una versión nueva: la versión en servicio sigue atendiendo hasta que se active otra, y cada evaluación queda sellada con la versión que la produjo.
- **Barandas numéricas** (edad, ingreso mínimo, deuda sobre ingreso, cuota sobre ingreso, techo de exposición, plazo máximo, tasa base) y **categorías A–E** con su decisión, spread de tasa, factor de exposición y plazo máximo.

Las barandas se aplican **después** del análisis y solo pueden restringir: bajan el límite, acortan el plazo, suben la tasa al piso de la categoría y endurecen la decisión. Nunca la aflojan. Cada ajuste queda registrado en la evaluación, así que un analista ve exactamente qué recortó la política.

**3. El cálculo de disponibilidad.** La capacidad de pago es el menor entre el excedente disponible y un porcentaje del ingreso reconocido (los ingresos sin verificar sufren un descuento configurable). Esa cuota se convierte en capital invirtiendo el mismo cronograma de amortización francesa que usan los préstamos reales, de modo que el límite recomendado siempre es un capital cuya cuota más alta cabe en la capacidad de pago.

**Motor de análisis.** Si `ANTHROPIC_API_KEY` está configurada, el prompt se ejecuta contra Claude con salida estructurada. Si no lo está —o si la llamada falla— la evaluación usa un **scorecard determinístico** del dominio que pondera capacidad de pago, endeudamiento, estabilidad laboral, buró e historial de pago interno. El módulo funciona completo sin credenciales de IA; el scorecard también se usa siempre para las precalificaciones y se le entrega al análisis por prompt como referencia. Cuando no hay datos de buró o historial, esos factores puntúan neutro en lugar de penalizar a quien no tiene legajo.

## Decisiones técnicas

- **shopspring/decimal** para toda aritmética monetaria (nunca `float64`)
- **Tasas de interés en porcentaje anual** - `CreditLine.InterestRate`,
  `Loan.InterestRate`, `RiskModel.BaseInterestRate` y `RiskGradeBand.InterestSpread`
  se guardan como porcentaje anual: `45` significa 45% anual. La conversión a tasa
  mensual divide por 12 meses y por 100 puntos (`monthlyRateDivisor = 1200` en
  `amortization.go`). Es la misma unidad que muestra y envía el frontend.
  No confundir con los *ratios* del modelo de riesgo (`MaxDTI`, `ExposureFactor`,
  `ProbabilityOfDefault`), que sí son fracciones (0.45 = 45%)
- **UUID** como primary key en todas las entidades
- **Soft deletes** via `gorm.DeletedAt`
- **Sin SQL directo** - solo API chainable de GORM, incluso para agregaciones del dashboard
- **Arquitectura hexagonal** - el dominio no depende de infraestructura
- **Auditoría por contexto** - middleware inyecta IP/UserAgent
- **JWT local** en desarrollo, Firebase Auth en producción
- **Almacenamiento local** como fallback de Google Cloud Storage
- **OTP por WhatsApp (Twilio)** - el canal se elige por configuración: con
  credenciales de Twilio el código va por WhatsApp, si no cae a email link de
  Firebase y finalmente a consola, así que el desarrollo local no necesita
  cuentas. El puerto `port.OTPSender` vive en el dominio; Twilio, Firebase y
  consola son adaptadores
- **Política de riesgo como dato** - el prompt y las barandas son entidades versionadas, editables sin deploy
- **Las barandas solo restringen** - una recomendación externa nunca puede ampliar el límite, el plazo ni bajar la tasa por debajo del piso de su categoría
- **Degradación sin IA** - sin `ANTHROPIC_API_KEY` el riesgo se evalúa con un scorecard determinístico del dominio

## Deploy a producción (GCP — proyecto `fedegan-prestia-30x`)

Este es el camino de deploy real: `.github/workflows/deploy.yml` se dispara con cada
push a `main`, construye la imagen (frontend + backend en `Dockerfile.cloud`), la sube a
Artifact Registry y despliega a Cloud Run. Un único servicio sirve la SPA de React
(`FRONTEND_DIR=/app/public`) y la API bajo `BASE_PATH=/demo`, así que el frontend es
*same-origin* con la API y no necesita CORS en producción.

**La landing de Astro (`static-site/`) no se deploya.** Queda como código del repo, sin
Firebase Hosting: no hay reverse proxy delante de Cloud Run y la app se accede
directamente en la URL del servicio.

| Recurso | Valor |
|---|---|
| Proyecto GCP | `fedegan-prestia-30x` |
| Región | `us-central1` |
| Servicio Cloud Run | `prestia-app` |
| Artifact Registry | `us-central1-docker.pkg.dev/fedegan-prestia-30x/docker-repo/prestia-app` |
| Instancia Cloud SQL | `fedegan-prestia-30x:us-central1:prestia-db` |

El ID del proyecto vive centralizado en el bloque `env:` de `.github/workflows/deploy.yml`
(`PROJECT_ID`, `IMAGE`, `SQL_INSTANCE`). Para apuntar a otro proyecto, cambiar ese bloque.

> **Atención en bases con datos previos:** las tasas de interés pasaron a
> guardarse como porcentaje anual (`45` = 45%). Una base que venga de una versión
> anterior con tasas en fracción (`0.45`) las interpretaría como 0.45% anual, es
> decir un crédito casi sin interés. En ese caso hay que multiplicar por 100
> `credit_lines.interest_rate`, `loans.interest_rate`,
> `risk_models.base_interest_rate` y `risk_grade_bands.interest_spread` antes de
> exponer el servicio. Con base nueva + `seed` no aplica.

### Infraestructura requerida (una sola vez, por proyecto)

Verificar que existan en `fedegan-prestia-30x` antes del primer deploy — los nombres de
recurso arriba son los esperados por la config, no se crean automáticamente:

```bash
gcloud config set project fedegan-prestia-30x
gcloud services enable run.googleapis.com artifactregistry.googleapis.com sqladmin.googleapis.com secretmanager.googleapis.com
gcloud artifacts repositories list --location=us-central1   # debe existir docker-repo
gcloud sql instances list                                   # debe existir prestia-db
```

### Secrets de GitHub Actions

Autenticación por Workload Identity Federation: `WIF_PROVIDER`, `WIF_SERVICE_ACCOUNT`.
Build del frontend: `VITE_FIREBASE_API_KEY`, `VITE_FIREBASE_AUTH_DOMAIN`,
`VITE_FIREBASE_PROJECT_ID`, `VITE_FIREBASE_STORAGE_BUCKET`,
`VITE_FIREBASE_MESSAGING_SENDER_ID`, `VITE_FIREBASE_APP_ID`, `VITE_FIREBASE_MEASUREMENT_ID`.

### Secrets de Secret Manager (los lee Cloud Run en runtime)

`DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE`, `DB_INSTANCE`,
`JWT_SECRET`, `FIREBASE_CREDENTIALS`.

## Deploy con Docker (self-hosted)

### Requisitos

- Servidor con Docker y Docker Compose
- PostgreSQL (managed o containerizado)
- Dominio configurado
- (Opcional) Firebase Auth, Google Cloud Storage

### Pasos

1. Clonar el repo en el servidor:
   ```bash
   git clone git@github.com:diogenes-moreira/creditos.git
   cd creditos
   ```

2. Configurar variables de producción:
   ```bash
   cp .env.example .env
   # Editar .env con valores de producción:
   # - JWT_SECRET fuerte y único
   # - GIN_MODE=release
   # - Credenciales reales de PostgreSQL
   # - Firebase y GCS si se usan
   ```

3. Construir y levantar:
   ```bash
   docker-compose up -d --build
   ```

4. Cargar datos iniciales (primera vez):
   ```bash
   docker exec -it creditos-backend ./seed
   ```

5. Verificar:
   ```bash
   curl http://localhost:8080/health
   # {"status":"ok"}
   ```

### Nginx reverse proxy (ejemplo)

```nginx
server {
    listen 80;
    server_name prestia.com.ar;

    location / {
        proxy_pass http://localhost:3000;
    }

    location /api/ {
        proxy_pass http://localhost:8080;
    }

    location /swagger/ {
        proxy_pass http://localhost:8080;
    }
}
```

## Licencia

MIT
