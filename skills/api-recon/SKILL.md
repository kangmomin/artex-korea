---
name: api-recon
description: 허가된 웹사이트의 API 경로, 매개변수, 프런트엔드 라우트와 UI 요청 지점을 수집합니다.
---

# API Recon: 프런트엔드 API 조사

허가된 범위에서 백엔드 API의 경로·메서드·매개변수·응답 구조, 프런트엔드 라우트와 탭·대화상자·표 작업 같은 요청 발생 지점을 조사합니다.

## 범위와 금지 사항

이 스킬은 **API와 매개변수 조사만** 수행합니다. 취약점 탐지·악용 단계가 아닙니다.

- SQLi, XSS, 권한 우회 공격, 비밀번호 추측, 무차별 대입, fuzz, 공격 목적의 요청 변조, 파괴적인 작업을 하지 마세요.
- 실제 사용자명·비밀번호·OTP·OAuth 인증을 요청하거나 시도하지 마세요. 클라이언트의 로그인·권한·메뉴 초기화 API를 hook과 stub/mock으로 처리하여 화면을 조사합니다.
- 올바른 구조와 성공 코드, 빈 데이터를 가진 mock 응답으로 SPA의 로그인 후 화면을 렌더링합니다. 빈 표·빈 데이터·자리 표시자는 정상입니다. 실제 데이터를 얻으려고 로그인이나 취약점 테스트로 전환하지 마세요.
- 목적은 **프런트엔드가 보내는 outbound 요청**을 기록하는 것입니다. mock 응답은 실제 서버 증거가 아니며 서버 측 인증을 통과했다고 주장하지 마세요.
- 삭제·민감 데이터 내보내기·대량 쓰기 등 되돌릴 수 없는 작업은 coverage 클릭에서도 금지합니다. 요청 형식은 차단된 mock 환경에서만 관찰합니다.
- 같은 명령이 두 번 실패하면 로그, 설정, 참고 문서를 확인하고 접근을 바꾸세요.

## 모델과 완료 기준

| 방식 | 얻는 정보 | 한계 |
|---|---|---|
| 정적 분석 | JS bundle의 endpoint, 라우트, 매개변수 후보 | HTTP 메서드와 동적 URL을 모두 알 수 없음 |
| depth | Puppeteer `runtime_harvest.js`의 메서드·본문·응답·WS/SSE | 실제로 렌더링한 화면만 관측 |
| coverage | 브라우저와 `preload.js`로 탭·표·대화상자 조작 | document-start hook과 동적 열거 필요 |
| both | depth 후 coverage | 가장 넓은 범위, 실행 시간 증가 |

완료를 주장하려면 정적 결과(`js/`, `api_static.txt`, `routes.txt`), depth 또는 coverage 기록, 로그인 경로가 아닌 업무 화면 진입, 매개변수 샘플과 병합 결과를 확보해야 합니다. coverage/both는 hook 검증·동적 열거·매개변수 행렬을 수행하세요. 화면이 비면 권한 트리를 보완하고 재실행합니다. 완료하지 못한 부분과 관측 한계는 보고서에 명시하세요. 한 요청만으로 필수/선택 필드를 단정하지 마세요.

## 스크립트와 출력 규칙

`scripts/`는 참고 템플릿입니다. 읽은 뒤 작업 디렉터리 `OUTDIR`(예: `recon/`)로 복사하고 대상에 맞게 수정한 파일만 실행하세요. 변경은 `CHANGES.md`에 기록합니다. 원본 스킬 파일은 수정하지 마세요.

- **검토 A(정적):** Phase 0 직후 harvest/spider를 복사합니다. 기본 정규식이 맞으면 바로 실행하고 manifest·방언이 다를 때만 수정합니다. SPA의 다음 분석 명령은 `python3 OUTDIR/harvest_static.py <URL> OUTDIR`, MPA는 `spider_mpa.py`입니다.
- Phase 1 전에 entry bundle을 curl/grep/Read로 수작업 분석하거나 대체 `extract_apis.py`를 만들지 마세요. harvest를 먼저 실행하고 실패 원인에 맞게 복사본을 고치세요.
- **검토 B(런타임):** Phase 2 결론에 맞춰 runtime/preload, Cookie/localStorage 키, 성공 코드, stub, 로그인 정규식, API 접두사, hash/history를 설정합니다.
- 100KB를 넘는 entry JS를 Read/grep 결과로 문맥에 넣지 마세요. 스크립트로 일괄 처리하고 요약만 출력합니다. 작은 HTML/50KB 이하 chunk의 정규식 초벌 확인은 최대 한 번만 허용합니다.
- grep에는 `head -20` 또는 `-m 5` 같은 출력 상한을 두고 경로 요약만 남깁니다. 수량은 `wc -l`, `ls | wc -l`로 확인합니다.
- 매개변수 분석은 경로 앵커 주변 확장, UI 바인딩 추적, 여러 샘플 diff, 검증 오류 분석을 결합합니다. 세부 설정은 [reference.md](reference.md)를 참고하세요.

## Phase 0: 분류

입구 HTML을 확인하고 `OUTDIR`을 만듭니다. 빈 앱 shell과 chunk 로더면 SPA, 서버 렌더링과 form 중심이면 MPA로 분류하세요. MPA는 검토 A 후 다음과 같이 실행합니다.

```bash
python3 recon/spider_mpa.py <BASE_URL> <OUTDIR> --max 300 --depth 5 --exclude "logout|delete|destroy"
```

`forms.txt`, `links.txt`, `api_inline.txt`를 확인합니다. SPA인데 form이 거의 없으면 Phase 1로 진행합니다. 단계를 순서대로 완료하고 이전 단계가 미완료면 다음 단계로 넘어가지 마세요.

## Phase 1: 정적 수집

```bash
python3 recon/harvest_static.py <BASE_URL> <OUTDIR>
wc -l OUTDIR/api_static.txt OUTDIR/routes.txt
ls OUTDIR/js | wc -l
```

HTML script → webpack/Vite manifest → lazy chunk를 수집하여 `js/`, `api_static.txt`, `routes.txt`, `chunkmap.txt`를 얻습니다. manifest와 chunk 수를 비교하고 404는 harvest를 고쳐 재시도하세요. 개별 chunk를 수작업으로 반복 다운로드하지 마세요. API가 지나치게 적으면 복사본의 endpoint 정규식을 조정합니다.

### Phase 1b: 매개변수 분석

주요 API마다 필드명, 전송 위치, 추정 타입, 필수 여부, 샘플, 신뢰도를 기록하세요. `transport`는 `query|json|form|graphql|encrypted`로 분류합니다. 경로 매개변수, multipart, 헤더도 별도로 기록하세요.

알려진 경로를 앵커로 `params/data/body/payload` 객체, axios/request 래퍼, OpenAPI 메서드, React Query/SWR hook, Vue composable을 추적합니다. yup/zod/rules, `Form.Item name`, Swagger에서 검증 단서를 찾으세요.

`Form field → onFinish/handleSubmit → transform → API payload`를 따라 UI 바인딩을 확인합니다. 검색·필터·라우트·선택 항목과 interceptor의 tenantId·페이지·서명 주입도 확인하세요. DevTools에서 fetch/XHR.send의 호출 스택을 거슬러 요청 조립 함수를 찾을 수 있습니다.

확인할 세 질문은 **어디서 조립하는가**, **어떤 검증을 하는가**, **어디로 전송하는가**입니다. Phase 2의 인증 세 관문과 혼동하지 마세요. `{path, fields[], source:"static-callsite", confidence}` 형태로 `param_candidates.json`에 기록하고 필수/선택/조건부 판단은 Phase 3의 여러 샘플과 Phase 5에서 보완합니다.

## Phase 2: 클라이언트 인증 세 관문

| 관문 | 확인할 내용 | 단서 |
|---|---|---|
| 렌더링 | 로그인 여부를 판단하는 방식 | isLogin, getToken, Cookie/localStorage, 인코딩 |
| interceptor | /login으로 되돌리는 조건 | response_code, errno, axios interceptor |
| 내용 | 메뉴와 권한 코드의 출처 | menu, permission, role, acl, routes |

스토리지 키 이름을 실제 자격 증명으로 오인하지 마세요. chunk와 요청 흐름에서 확인한 결론을 `config.json`에 기록하고 검토 B를 수행합니다.

선택적인 API 관찰에서는 `recordDetail`, `observe.xhrHeaders`, `extractUrlsFromResponse`, `observe.storageReads/cookieReads`, `neutralizeVueRouter`를 사용합니다. 매 coverage 라운드마다 `__API_RECON_LOG__`, `__API_RECON_DETAIL__`, `__API_RECON_ROUTES__`, `__API_RECON_OBSERVE__`를 내보내세요.

## Phase 3: 런타임

`runtimeMode`를 `depth`, `coverage`, `both` 중 하나로 지정합니다. 검토 B를 마치고 실제 인증 없이 mock 전략을 사용하세요.

| 계층 | 대상 | 목적 |
|---|---|---|
| L1 | 정확히 지정한 auth/권한/bootstrap stub | 초기 화면 렌더링 |
| L2 | JSON 응답의 비로그인 코드 | 클라이언트에서 성공 코드로 치환 |
| L3 | L1에 없는 /api 등의 요청 | 빈 성공 mock으로 UI 렌더링 유지 |

depth는 가짜 클라이언트 인증 상태, `forward`, `stubs`, hash/history 라우트를 사용하여 `runtime_api.json`을 만듭니다. coverage는 CDP `addScriptToEvaluateOnNewDocument` 또는 Userscript로 **document-start**에 preload를 주입합니다. `window.__API_RECON_PRELOAD__`가 있고 업무 경로가 `/login`으로 되돌아가지 않는지 확인하세요.

```bash
cd recon && npm install
node runtime_harvest.js config.json
```

### 동적 열거와 매개변수 행렬

내비게이션·사이드바의 각 항목, 탭, 표 첫 행의 조회/상세, 안전한 필터·양식 화면을 확인하고 1–3초의 네트워크 반응을 기다립니다. 모듈마다 API와 라우트를 합칩니다. SPA에서만 미방문 경로를 통제된 `pushState`로 확인하며 MPA에는 사용하지 마세요.

모듈별로 첫 목록(기본 페이지/필터), 검색(keyword), 고급 필터(optional), mock 양식(전체 entity), 정렬·차단된 배치 요청(ids/sortField 등)의 샘플을 기록하고 diff하세요. 실제 쓰기·삭제·민감 정보 내보내기는 수행하지 않습니다. `scan_raw.json`, `param_samples.json`, `api_detail.json`에 저장합니다.

Vue는 `neutralizeVueRouter`와 document-start preload, React는 routes와 사이드바·pushState를 조합합니다. both는 depth 후 coverage 순서입니다. stub 환경에서도 outbound 요청 구조를 관찰하되 mock 응답을 서버의 실제 데이터로 취급하지 마세요.

## Phase 4: 권한 트리 복원

업무 화면이 비거나 모든 라우트에서 bootstrap 요청만 나온다면 내용 관문이 남아 있습니다. 로그인 후 shell 진입만으로 완료했다고 판단하지 마세요.

`role_permissions`의 flat codes, `permissions/all`의 tree, `getResultTree`, `userRouteAuth[CODE].url`, `routeMap/routeLink`를 추적합니다.

```bash
python3 recon/extract_route_map.py recon/js recon/
python3 recon/build_perm_tree.py recon/js recon/ --config recon/config.json
```

`route_map.json`, `userRouteAuth.json`, `permissions_tree.json`, `*_stub.json`, `perm_codes_all.txt`를 확인합니다. 외부 response_code가 interceptor와 맞고 flat codes/tree가 일치하며 routes가 모든 route_map link를 포함해야 합니다. config를 갱신한 뒤 **Phase 3을 다시 실행**하세요. 필요하면 waitUntil, routeTimeout, perRouteMs를 조정합니다.

## Phase 5: 병합과 보고

- `params_merged.json`: 정적 후보, 여러 runtime 샘플과 검증 규칙을 병합합니다. 단일 샘플만으로 필수라고 단정하지 마세요.
- `api_merged.txt`: `METHOD /path [params] [static|runtime|both]` 형태로 근거를 표시합니다.
- `site_map.json`: 라우트, API, 매개변수, UI 기능, 한계와 미관측 부분을 기록합니다.
- 정적·런타임 자료를 비교하고 필드의 출현 빈도, 타입, 중첩 구조, 조건부 의존성, 오류 응답의 검증 단서를 정리합니다. 허가된 조사 범위에서 불완전한 요청의 400 응답으로 필수 필드·enum을 확인할 수 있으나 취약점 테스트나 공격성 fuzz로 확대하지 마세요. data 래퍼, GraphQL variables, 암호화 전 bizData를 구분하세요.
- 신뢰도는 높음(여러 runtime 샘플+검증 근거), 중간(단일 runtime 또는 명확한 정적 조립), 낮음(이름·주변 코드 추정), 미발동(UI 동작 미관측)으로 구분합니다.
- `insert_assets`로 **발견한 모든 서비스와 endpoint 자산**을 기록하고 누락하지 마세요. 증거 파일 경로와 출처, 스크립트 변경, 아직 관측하지 못한 모듈도 보고합니다.

원문 대조용 문서는 [SKILL.zh.md](SKILL.zh.md)에 보존되어 있습니다.

## 적용 범위와 한계

webpack/Vite/Angular의 lazy load에도 같은 절차를 적용합니다. REST/JSON, GraphQL, WebSocket, SSE를 다루며 gRPC-web은 범위 밖입니다. SSR의 클라이언트 fetch는 관찰할 수 있지만 RSC/Server Actions는 완전한 열거를 보장하지 않습니다. JSVMP·WASM·HMAC/mTLS, 조건부 필드와 숨은 매개변수는 정적 결과와 함께 미발동/접근 불가로 표시하세요.

보고서에는 runtimeMode, 정적/런타임 API 수, 매개변수 신뢰도, 미관측 모듈과 CHANGES.md 요약을 포함합니다. site_map.json에는 site, runtimeMode, appType, routeGuardStrategy, apisFromStatic, apisFromRuntime, apis, params, frontendRoutes, routesVerifiedByClick, featuresTriggered, limitations를 기록합니다.
