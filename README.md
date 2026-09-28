# Agent Control Plane

**여러 컴퓨터와 AI 에이전트 사이에서 작업을 이어가기 위한, 사용자 소유의 작업 기록 서버입니다.**

현재 구현된 모듈은 **Context Hub**입니다. 에이전트가 작성한 작업 요약, 결정 사항, 진행 기록, 인계 내용을 저장하고 HTTP API로 제공합니다. 새로운 에이전트는 이 기록을 읽어 작업의 배경과 다음 할 일을 파악할 수 있습니다.

Codex, Claude, Antigravity 등 특정 제품의 대화 형식이나 SDK에 의존하지 않습니다. 각 에이전트 또는 연동 도구가 인증된 HTTP 요청을 보낼 수 있으면 같은 API를 사용할 수 있습니다. 제품별 자동 연결 기능은 별도의 연동이 필요합니다.

- 운영 서버: <https://agent.gwinam.com>
- API 명세: [OpenAPI](https://agent.gwinam.com/openapi.json)
- 연결 승인: [에이전트 연결](https://agent.gwinam.com/activate)
- 연결 관리: [연결된 기기](https://agent.gwinam.com/devices)

## 서버의 역할

Context Hub는 작업 데이터를 **인증·검증·저장·조회하는 인터페이스**입니다. 작업을 수행하는 주체는 각 컴퓨터에서 실행되는 에이전트입니다.

| Context Hub                     | 에이전트와 로컬 도구         |
| ------------------------------- | ---------------------------- |
| 작업과 세션 기록 저장           | 저장소와 현재 작업 상태 확인 |
| 작업 목록·개요·세션별 기록 제공 | 코드 수정, 테스트, 빌드 실행 |
| 접근 권한 확인과 감사 기록      | 저장할 요약과 결정 사항 작성 |
| 변경 충돌과 중복 요청 처리      | 다음 행동 판단과 작업 인계   |

서버는 에이전트나 LLM을 실행하지 않고, 작업을 스스로 시작하거나 기록을 자동 요약하지 않습니다. 대화 원문 대신 에이전트가 명시적으로 작성한 구조화된 기록을 저장합니다. 프로젝트의 실행 환경 자격 증명도 맡기지 않습니다.

향후 Agent Control Plane 전체에는 스킬·워크플로 레지스트리나 실행 조정 기능을 추가할 수 있습니다. 이러한 기능은 현재 Context Hub의 구현 범위에 포함되지 않습니다.

## 빠른 시작: 다른 컴퓨터에서 연결하기

운영 서버를 이용하는 클라이언트에 Go, Docker, 데이터베이스를 설치할 필요는 없습니다. 아래 예시는 **macOS 또는 Linux, Python 3, Git**을 기준으로 합니다. Windows에서는 WSL 등 Linux 환경을 사용하세요.

현재 제공하는 Python 인증 보조 스크립트는 표준 라이브러리만 사용하며 컴파일이 필요 없습니다. API 자체는 이 스크립트나 저장소 설치에 종속되지 않습니다. 자체 자격 증명 저장소를 갖춘 클라이언트는 [기기 인증 API](docs/device-authorization.md)를 직접 구현할 수 있습니다.

### 1. 인증 보조 스크립트 준비

```sh
git clone https://github.com/HyeonTee/agent-control-plane.git ~/agent-control-plane
```

이미 저장소가 있다면 해당 경로를 사용합니다. 아래 명령의 `~/agent-control-plane`은 실제 저장 위치에 맞게 변경하세요.

### 2. 연결 코드 발급

```sh
python3 ~/agent-control-plane/scripts/hub-credentials.py start \
  --label "Claude on my laptop" \
  --scope "context:read work:write"
```

`--label`에는 승인 화면에서 알아볼 수 있는 기기나 클라이언트 이름을 입력합니다. 조회만 필요하면 `--scope "context:read"`를 사용합니다.

명령은 코드가 미리 채워진 승인 주소(`https://agent.gwinam.com/activate?code=...`)를 출력하고 기본 브라우저에서 엽니다. 브라우저를 열지 않으려면 `--no-browser`를 추가합니다.

1. GitHub로 로그인합니다. 로그인 후 해당 코드의 승인 화면으로 바로 돌아옵니다.
2. 화면의 코드가 터미널에 표시된 일회용 코드와 같은지 확인합니다.
3. 클라이언트 이름과 요청 권한을 확인하고 승인합니다.

`/activate`에 직접 접속한 경우에는 코드를 먼저 입력하면 됩니다. 대소문자, 공백, 하이픈은 구분하지 않습니다.

코드는 10분 동안 유효합니다. 서버에 등록된 **소유자의 GitHub 숫자형 사용자 ID**와 일치하는 계정만 승인할 수 있습니다. 다른 계정은 403 오류 화면과 함께 거부됩니다.

### 3. 연결 완료 및 작업 목록 조회

코드를 발급받은 컴퓨터에서 실행합니다.

```sh
python3 ~/agent-control-plane/scripts/hub-credentials.py finish

python3 ~/agent-control-plane/scripts/hub-credentials.py api GET \
  '/api/v1/tasks?status=active&limit=20'
```

자격 증명은 기본적으로 `~/.config/agent-control-plane/credentials.json`에 소유자만 접근할 수 있는 권한으로 저장됩니다. `XDG_CONFIG_HOME`이 설정되어 있으면 그 아래에 저장됩니다. 이후 API 호출 시 스크립트가 토큰을 갱신하므로 매번 브라우저 인증을 할 필요가 없습니다.

이 파일은 운영체제 키체인이 아닌 로컬 파일입니다. 토큰 파일을 에이전트 대화에 첨부하거나 출력하지 마세요. GitHub OAuth App의 Client Secret은 서버 설정이며, 클라이언트 컴퓨터에 복사하지 않습니다.

### 4. 에이전트에게 작업 이어받기 요청

작업할 프로젝트에서 Claude Code 등 로컬 명령을 실행할 수 있는 에이전트를 열고 다음과 같이 요청합니다.

```text
https://agent.gwinam.com에 저장된 작업을 이어서 진행해줘.
API 명세는 https://agent.gwinam.com/openapi.json에 있어.

인증이 필요한 API 요청은 다음 스크립트로 실행해:
python3 ~/agent-control-plane/scripts/hub-credentials.py api METHOD '/api/v1/...'

먼저 진행 중인 작업 목록을 보여줘.
내가 선택한 작업의 개요와 최신 handoff를 읽고,
필요한 세션 기록을 조회한 뒤 현재 프로젝트 상태와 대조해줘.
작업을 진행하면서 세션과 체크포인트를 기록하고,
마칠 때 완료 내용과 남은 일을 handoff로 저장해줘.
대화 원문과 비밀 값은 저장하지 말고,
로컬 인증 파일이나 토큰 값을 읽거나 출력하지 마.
```

작업 기록은 배경 정보를 전달합니다. 실제 코드를 수정하려면 해당 컴퓨터에 작업 대상 저장소와 개발 환경도 준비되어 있어야 합니다.

## 작업 기록 구조와 조회 순서

```text
공간(Space): 접근 권한의 범위
└── 프로젝트(Project): 작업 대상과 동기화 정책
    └── 작업(Task): 목표와 현재 상태
        ├── 작업 개요와 최신 인계
        └── 세션(Session): 한 클라이언트의 작업 단위
            └── 체크포인트·인계 기록
```

체크포인트는 진행 상황을 남기는 기록이며, `handoff`는 다음 에이전트가 이어받을 수 있도록 남은 일 등을 포함하는 인계 기록입니다. 새 기록을 추가하는 방식으로 이력을 유지합니다. 세션 도입 이전의 기록은 별도 조회 경로로 제공됩니다.

전체 기록을 처음부터 모두 읽을 필요는 없습니다. 작업 목록에서 대상을 고른 뒤 개요, 세션 목록, 필요한 세션의 상세 기록 순서로 조회합니다.

| 단계           | API                                                                  | 용도                                   |
| -------------- | -------------------------------------------------------------------- | -------------------------------------- |
| 작업 목록      | `GET /api/v1/tasks?status=active&limit=20`                           | 진행 중인 작업과 요약 확인             |
| 작업 개요      | `GET /api/v1/tasks/{task_id}`                                        | 목표, 버전, 최근 기록과 최신 인계 확인 |
| 세션 목록      | `GET /api/v1/tasks/{task_id}/sessions?limit=20`                      | 이전 세션의 요약과 인덱스 확인         |
| 세션 기록      | `GET /api/v1/tasks/{task_id}/sessions/{session_id}/entries?limit=50` | 선택한 세션의 상세 기록 조회           |
| 이전 형식 기록 | `GET /api/v1/tasks/{task_id}/pre-session-history`                    | 세션에 연결되지 않은 과거 기록 조회    |

목록 응답에 `next_cursor`가 있으면 다음 요청의 `cursor`에 전달합니다. 모든 세션을 읽으려면 세션 목록과 각 세션의 기록 모두에서 페이지를 끝까지 조회해야 합니다.

작업 기록을 남기는 기본 순서는 다음과 같습니다.

1. 작업에 새 세션을 생성합니다.
2. 진행 중 체크포인트를 추가합니다.
3. 마무리할 때 남은 일이 포함된 `handoff`를 추가합니다.
4. 세션 요약과 함께 세션을 종료합니다.

체크포인트에는 조회한 작업의 `expected_version`과 요청별 `Idempotency-Key`를 사용합니다. 같은 요청을 재시도할 때는 키와 본문을 그대로 유지합니다. 작업 버전이 오래되었거나 같은 키에 다른 본문을 보내면 `409`가 반환됩니다. 상세 요청 형식은 [저장소의 OpenAPI 명세](api/openapi.json)를 참고하세요.

자동 요약이나 compact 기능은 아직 없습니다. 향후 도입하더라도 요약을 작성하는 주체와 원본 이력 보존 정책을 별도로 설계해야 합니다.

## 인증과 접근 권한

GitHub는 브라우저에서 승인하는 사람의 신원을 확인하는 용도로 사용합니다. 에이전트의 API 접근에는 Hub가 발급한 별도 자격 증명을 사용합니다.

```text
클라이언트 → Hub: 연결 코드 요청
사용자 → 브라우저: GitHub 로그인 후 코드와 권한 승인
클라이언트 → Hub: 승인 결과를 확인하고 자격 증명 저장
클라이언트 → Hub: 인증된 API 요청과 토큰 갱신
```

- **권한:** `context:read`는 조회, `work:write`는 작업 기록 작성에 사용합니다.
- **접근 범위:** 현재 브라우저 승인은 초기 설정된 소유자의 `personal` 공간을 대상으로 합니다.
- **만료:** 접근 토큰은 15분, 갱신 자격 증명은 마지막 사용 후 90일 동안 유효합니다.
- **갱신:** 갱신 시 토큰이 교체됩니다. 이미 사용한 갱신 토큰을 다시 사용하면 해당 연결이 폐기됩니다.
- **연결 해제:** 소유자가 [연결된 기기](https://agent.gwinam.com/devices)에서 접근과 갱신을 취소할 수 있습니다.

GitHub 로그인 세션은 30분 동안 유지되며 Hub 재시작 시 무효화됩니다. 클라이언트의 API 토큰 갱신과는 별개입니다. GitHub 토큰은 사용자 확인 후 보관하지 않습니다.

API 명세는 공개되지만 작업 데이터에는 인증이 필요합니다. 자격 증명은 프롬프트·로그·저장소에 포함하지 않습니다. 프로젝트별 동기화 정책과 데이터 경계는 [보안 문서](docs/security.md)를 참고하세요.

## 아키텍처와 기술 스택

```text
각 컴퓨터
  에이전트 ── 로컬 도구 ── 작업 저장소
      │
      │ 인증된 REST/JSON 요청
      ▼
CloudFront / AWS WAF
      │ HTTPS
      ▼
EC2 · Docker Compose
  Caddy → Go Context Hub → PostgreSQL
                               │
                               └── S3 백업
```

| 구성                                  | 역할                                            |
| ------------------------------------- | ----------------------------------------------- |
| Go                                    | HTTP API, 인증, 기록 검증과 저장 처리           |
| PostgreSQL 17                         | 작업·세션·권한·감사 이력 저장                   |
| REST/JSON 및 OpenAPI                  | 에이전트와 연동 도구가 사용하는 공개 인터페이스 |
| Python 3                              | 선택적 로컬 인증 보조 스크립트                  |
| Docker Compose / Caddy                | 서버 실행과 HTTPS 처리                          |
| AWS EC2 / CloudFront / WAF / S3 / SSM | 운영 환경, 요청 제한, 백업, 설정 관리           |
| OpenTofu                              | AWS 인프라 정의                                 |

서버는 도메인, 애플리케이션, HTTP·PostgreSQL 어댑터를 분리한 단일 애플리케이션입니다. Go 버전과 의존성은 [go.mod](go.mod)와 [go.sum](go.sum)에서 관리합니다. 현재 큐, 벡터 데이터베이스, 에이전트 실행 엔진, 원격 MCP 서버는 사용하지 않습니다.

## 로컬 개발

### 서버 실행

Docker Compose로 개발용 PostgreSQL과 Hub를 실행합니다.

```sh
docker compose up --build -d
curl -i http://127.0.0.1:8081/health
curl -i http://127.0.0.1:8081/ready
curl http://127.0.0.1:8081/openapi.json
```

Hub는 `127.0.0.1:8081`, PostgreSQL은 `127.0.0.1:15433`에 바인딩됩니다. `/health`는 프로세스 상태를 확인하고, `/ready`는 데이터베이스 연결이 정상일 때 `204`를 반환합니다. 서버 시작 시 SQL 마이그레이션을 적용합니다.

로컬 Compose의 데이터베이스 비밀번호는 개발 전용입니다. 기본 로컬 구성에는 GitHub OAuth 설정이 없으므로 아래 관리자 토큰으로 API를 사용합니다.

### 최초 소유자와 토큰 생성

빈 개발 데이터베이스에서 한 번 실행합니다.

```sh
docker compose run --rm -T hub admin bootstrap \
  -space personal -token-name my-agent
```

출력되는 토큰 비밀 값은 한 번만 제공됩니다. 비밀번호 관리자 등에 보관하고, 다음 명령으로 셸 기록에 남기지 않고 입력합니다. 아래 예시는 Bash 또는 Zsh 기준입니다.

```sh
read -rs HUB_TOKEN
curl -H "Authorization: Bearer $HUB_TOKEN" \
  http://127.0.0.1:8081/api/v1/spaces
```

### 프로젝트와 작업 기록 생성

`SPACE_ID`, `PROJECT_ID`, `TASK_ID`, `SESSION_ID`는 앞선 API 응답에서 받은 실제 ID로 바꿉니다. 새 작업의 초기 버전은 `1`이며, 기존 작업이라면 현재 버전을 조회해 사용합니다.

```sh
curl -X POST http://127.0.0.1:8081/api/v1/spaces/SPACE_ID/projects \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"name":"my-project","sync_policy":"metadata_and_handoffs"}'

curl -X POST http://127.0.0.1:8081/api/v1/projects/PROJECT_ID/tasks \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"title":"첫 작업","objective":"작업 기록을 저장하고 다음 세션에서 이어받기"}'

curl -X POST http://127.0.0.1:8081/api/v1/tasks/TASK_ID/sessions \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"client_label":"my-agent"}'

curl -X POST http://127.0.0.1:8081/api/v1/tasks/TASK_ID/checkpoints \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -H 'Idempotency-Key: first-handoff-001' \
  -d '{"session_id":"SESSION_ID","expected_version":1,"kind":"handoff","summary":"작업 환경 준비 완료","remaining":["다음 구현 단계 진행"]}'

curl -X POST http://127.0.0.1:8081/api/v1/tasks/TASK_ID/sessions/SESSION_ID/close \
  -H "Authorization: Bearer $HUB_TOKEN" -H 'Content-Type: application/json' \
  -d '{"summary":"인계 기록 저장 완료"}'
```

관리자는 별도 클라이언트 토큰을 발급하거나 폐기할 수도 있습니다. 관리자 명령은 데이터베이스 접근 권한이 필요한 운영 도구이며 공개 API로 제공되지 않습니다.

```sh
docker compose run --rm -T hub admin issue-token \
  -space-id SPACE_ID -name second-agent -scopes context:read -days 30

docker compose run --rm -T hub admin revoke-token -client-id CLIENT_ID
```

### 테스트

```sh
go vet ./...
go test ./...
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_*.py'
```

PostgreSQL 통합 테스트는 `HUB_TEST_DATABASE_URL`이 있어야 실행됩니다. 별도의 `hub_test` 데이터베이스를 만들고 연결 문자열을 설정하세요. 테스트는 소유자 초기 설정과 데이터 변경을 수행하므로 실제 데이터가 있는 데이터베이스를 지정하지 않습니다. CI는 전용 PostgreSQL을 준비해 통합 테스트와 빌드를 실행합니다.

Compose 없이 서버를 실행하려면 `HUB_DATABASE_URL`을 설정하고 `go run ./cmd/hub`를 실행합니다. HTTP 주소는 `HUB_HTTP_ADDR`로 변경할 수 있으며 기본값은 `:8080`입니다.

## 자체 서버 설정과 배포

운영 배포는 기존 EC2에서 별도 Compose 프로젝트로 실행합니다. [수동 배포 워크플로](.github/workflows/deploy.yaml)는 검증, ARM64 이미지 빌드, EC2 적용, 백업·복원 확인과 공개 HTTPS 상태 확인을 수행합니다. 실제 배포 절차는 [운영 배포 문서](docs/deployment-existing-ec2.md)에 정리되어 있습니다.

브라우저 기기 인증을 활성화하려면 최초 소유자와 `personal` 공간을 초기 설정한 뒤 다음 값을 설정합니다.

| 환경 변수                  | 내용                                         |
| -------------------------- | -------------------------------------------- |
| `HUB_PUBLIC_URL`           | 공개 HTTPS 주소                              |
| `HUB_GITHUB_CLIENT_ID`     | GitHub OAuth App의 Client ID                 |
| `HUB_GITHUB_CLIENT_SECRET` | GitHub OAuth App의 Client Secret             |
| `HUB_GITHUB_OWNER_ID`      | 연결을 승인할 GitHub 계정의 숫자형 사용자 ID |

현재 운영 서버의 GitHub OAuth 콜백 주소는 `https://agent.gwinam.com/auth/github/callback`입니다. 운영 배포 스크립트는 `/agent-control-plane/github_client_secret`, `/agent-control-plane/github_owner_id`, `/agent-control-plane/github_client_id`를 SSM에서 읽습니다. Client Secret은 `SecureString`으로 저장하고, 기능 활성화 기준인 Client ID를 마지막에 생성합니다. 자세한 설정과 인증 프로토콜은 [기기 인증 문서](docs/device-authorization.md)를 참고하세요.

## 구현 현황

2026년 9월 28일 기준입니다.

| 구분                                                          | 상태                      |
| ------------------------------------------------------------- | ------------------------- |
| 공간·프로젝트·작업·세션 관리                                  | 구현 완료                 |
| 체크포인트·인계 저장과 단계별 조회                            | 구현 완료                 |
| 버전 충돌 감지·멱등 재시도·읽기/쓰기 감사 기록                | 구현 완료                 |
| GitHub 소유자 로그인과 브라우저 기기 승인                     | 구현 및 운영 배포 완료    |
| 접근 토큰 갱신과 연결 폐기                                    | 구현 완료                 |
| HTTPS·요청 제한·S3 백업·백업 복원 확인                        | 운영 적용 완료            |
| Codex 기록을 Claude Code에서 조회하고 후속 기록 작성          | 동일 컴퓨터에서 확인 완료 |
| 실제 브라우저 승인부터 다른 컴퓨터의 작업 재개까지            | 종단 간 확인 필요         |
| EC2 전체 재구축과 복구 훈련                                   | 후속 운영 작업            |
| 자동 컨텍스트 조립·스킬 레지스트리·compact·원격 MCP·실행 조정 | 미구현, 향후 검토         |

## 저장소 구조

```text
.
├── cmd/hub/                    # API 서버와 관리자 명령
├── internal/
│   ├── domain/                 # 작업과 인증 도메인 모델
│   ├── application/            # 애플리케이션 유스케이스
│   ├── adapter/
│   │   ├── httpapi/            # HTTP 인터페이스
│   │   └── postgres/           # 저장소 구현과 SQL 마이그레이션
│   └── config/                 # 서버 설정
├── api/                        # OpenAPI 명세
├── scripts/                    # 인증 보조 스크립트와 테스트
├── deploy/prod/                # 운영 Compose, HTTPS, 백업·복원
├── .github/workflows/          # CI와 수동 배포
├── compose.yaml                # 로컬 개발 환경
├── Dockerfile
├── CONTEXT.md                  # 프로젝트 용어와 역할
└── docs/                       # 설계와 운영 문서
```

## 관련 문서

- [프로젝트 용어와 역할](CONTEXT.md)
- [아키텍처](docs/architecture.md)
- [도메인 모델](docs/domain-model.md)
- [작업 탐색과 세션 기록](docs/work-discovery.md)
- [브라우저 기기 인증](docs/device-authorization.md)
- [보안과 데이터 경계](docs/security.md)
- [운영 배포](docs/deployment-existing-ec2.md)
- [최초 운영 인계 기록](docs/production-handoff.md)
- [로드맵](docs/roadmap.md)
- [아키텍처 결정 기록](docs/adr/README.md)
