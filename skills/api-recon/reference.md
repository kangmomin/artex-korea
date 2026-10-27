# API Recon 참고 문서

스크립트는 템플릿입니다. OUTDIR에 복사한 뒤 대상에 맞게 수정하세요. 현재 작업 순서와 범위는 [SKILL.md](SKILL.md)를 따릅니다. 원문 전체는 [reference.zh.md](reference.zh.md)에 보존되어 있습니다.

## A. 클라이언트 인증 세 관문

렌더링 상태의 저장 키·컨테이너·인코딩, 비로그인 응답을 처리하는 interceptor의 필드와 성공 값, 메뉴·권한 코드의 구조를 각각 확인하세요. 실제 로그인이나 서버 측 인증 우회는 수행하지 않습니다. 아래 패턴의 중국어는 기존 앱의 응답을 찾기 위한 검색 토큰이며 번역 대상이 아닙니다. 가짜 상태는 클라이언트 렌더링 조사에만 사용하고 실제 데이터 접근 권한으로 간주하지 마세요.

```bash
grep -rhoaE '.{0,40}(isLogin|isAuthenticated|loggedIn|hasLogin|requireAuth)\b.{0,80}' js | head
grep -rhoaE 'function (getUser|getToken|getAuth)[0-9]?\([^)]*\)\{.{0,200}' js | head
grep -rhoaE '(localStorage|sessionStorage)\.getItem\("[^"]+"\)' js | sort -u
grep -rhoaE '(Cookies?|cookie)\.(get|load)\("[^"]+"\)' js | sort -u
grep -rhoaE '\batob\(|JSON\.parse\(|jwt|decode' js | head
```

```bash
grep -rhoaE '.{0,60}(interceptors\.response|axios|request\.use).{0,120}' js | head
grep -rhoaE '.{0,40}(response_code|errcode|errno|\bcode\b|\bret\b|\bstatus\b)\s*[=!]==?\s*[\-0-9]{1,4}.{0,60}' js | head -20
grep -rhoaE '.{0,40}(未登录|请重新登录|登录已过期|unauthorized|登录失效|授权|token.{0,10}invalid).{0,40}' js | head
grep -rhoaE '.{0,30}(location\.href|router\.(push|replace)|navigate)\([^)]*login[^)]*\)' js | head
```

```bash
curl -sk -X POST -H 'Cookie: <fakekey>=junk' https://target/api/<protected> -d '{}' -H 'Content-Type: application/json'
```

```bash
grep -rhoaE '"/api[^"]*(permission|perm|role|menu|acl|resource|nav)[^"]*"' js | sort -u
grep -rhoaE '.{0,30}(menus|permissions|menuList|routeList|authList|role_permissions)\b.{0,120}' js | head
grep -rhoaE 'userRouteAuth|getResultTree|routeMap|routeLink|hasPermission|checkAuth' js | head
grep -rhoaE '([A-Z_][A-Z0-9_]*):\{name:"[^"]*",link:"/[^"]+"\}' js | head
```

## B. config.json 템플릿

이 템플릿의 키는 runtime 스크립트의 API입니다. 대상에서 확인한 키·라우트·성공 코드·stub 구조로 바꾸세요. role_permissions의 flat codes와 permissions/all의 트리는 같은 권한 집합을 나타내야 합니다. config와 수정 스크립트는 OUTDIR에 저장하세요.

```json
{
  "baseUrl": "https://target/",
  "runtimeMode": "both",
  "chromium": "/usr/bin/chromium",

  "cookies": [
    { "name": "auth", "value": "b64json:{\"id\":1,\"username\":\"admin\",\"role\":\"admin\",\"func\":{},\"permissions\":[\"*\"]}" }
  ],
  "localStorage": { "token": "faketoken", "isLogin": "1" },

  "neutralize": {
    "fields": ["response_code", "code", "errno", "ret", "status"],
    "success": 0,
    "flags": { "success": true, "message": "ok" }
  },
  "forward": true,
  "loginUrlPattern": "/login",
  "apiPattern": "/api/|/rest/|/graphql",

  "mockTier": "L1+L2",
  "recordDetail": true,
  "observe": {
    "storageReads": false,
    "cookieReads": false,
    "xhrHeaders": true
  },
  "neutralizeVueRouter": true,
  "stubs": [
    {
      "match": "permissions/all|/menu|role_permissions",
      "body": {
        "response_code": 0, "code": 0,
        "data": {
          "permissions": ["*"],
          "menus": [
            { "name": "dashboard", "path": "/dashboard", "show": true, "children": [] },
            { "name": "alert", "path": "/alert", "show": true, "children": [] }
          ]
        }
      }
    }
  ],

  "explore": {
    "clickTabs": true,
    "clickTables": true,
    "pushStateFallback": true,
    "maxMenuItems": 50
  },

  "routes": ["/dashboard", "/alert", "/asset", "/device", "/report", "/config", "/system"],
  "waitMs": 1500, "perRouteMs": 900, "headless": true,
  "waitUntil": "domcontentloaded",
  "routeTimeout": 12000,
  "proxy": "",

  "captureResponses": true, "recordWs": true, "respMax": 600
}
```

```json
"stubs": [
  {
    "match": "role_permissions",
    "body": {
      "response_code": 0,
      "data": {
        "permissions": ["MONITOR", "MONITOR_ALERT", "THREAT", "ASSETS_RISK"],
        "role_type": "SUPER_ADMIN"
      }
    }
  },
  {
    "match": "permissions/all",
    "body": {
      "response_code": 0,
      "data": [
        {
          "code": "MONITOR",
          "position": 1,
          "children": [
            { "code": "MONITOR_ALERT", "position": 1, "children": [] }
          ]
        }
      ]
    }
  }
]
```

## C. coverage preload 설정

preload를 문서 시작 전에 주입하세요. L1은 정확한 bootstrap stub, L2는 클라이언트의 실패 응답 코드 치환, L3는 빈 성공 mock입니다. Hook 활성화, 로그인 후 화면 렌더링, 각 모듈의 실제 요청 발생을 따로 확인하세요.

```javascript
const CONFIG = {
  loginPathRe: /\/(login|signin)(\/|$|\?)/i,
  mockTier: 'L1+L2',
  forward: true,
  recordDetail: true,
  extractUrlsFromResponse: true,
  neutralizeVueRouter: true,
  observe: { storageReads: false, cookieReads: false, xhrHeaders: true },
  neutralize: { fields: ['response_code', 'code'], success: 0 },
  stubs: [ /* config.json의 stubs와 동일 */ ],
  apiPattern: /\/(api|apis|v\d+|dev|internal|graphql)\//i,
};
```

```javascript
JSON.stringify({
  apis: [...window.__API_RECON_LOG__],
  detail: window.__API_RECON_DETAIL__,
  routes: [...(window.__API_RECON_ROUTES__ || [])],
  observe: window.__API_RECON_OBSERVE__,
}, null, 2)
```

## D. preload와 runtime hook 기능

fetch/XHR 요청·헤더, 스토리지·Cookie 읽기, Vue 라우트, WebSocket/SSE와 필요한 암호화 입력을 관찰합니다. 기록은 조사 범위의 요청 구조를 파악하는 용도입니다. 원래 코드의 기술 식별자와 증거를 보존하세요.

## E. 정적 endpoint 추출 정규식

Phase 1 결과가 지나치게 적을 때만 복사한 harvest의 정규식을 조정하고 재실행합니다. 미니파이 bundle 전체를 문맥에 넣지 마세요. 아래 예시는 출력 상한과 함께 사용하세요.

```bash
grep -rhoaE '"/[a-z][A-Za-z0-9_/\-]{3,}"' js | sort -u
grep -rhoaE '/api/[a-zA-Z0-9_./-]+' js | sort -u
```

## F. 문제 해결

API 수가 적으면 manifest와 lazy chunk 수·404·publicPath를 확인합니다. /login으로 돌아가면 저장 상태의 인코딩과 interceptor의 성공 필드를 확인합니다. shell은 보이는데 모듈이 비면 메뉴/권한 stub을 보완합니다. 매 라우트에서 같은 bootstrap만 보이면 권한 조건과 지연 로딩을 점검합니다. 실패한 동일 명령을 반복하지 말고 tool_logs와 수정 내용을 확인하세요.

## G. 강화된 대상의 한계

암호화·서명·서버 세션 의존성 때문에 클라이언트 mock만으로 렌더링할 수 없으면 정적 조사로 확인된 범위와 런타임 한계를 명시합니다. 실제 인증·취약점 공격으로 범위를 넓히지 마세요.

## H. 작업 완료 확인

정적 결과, 런타임 hook, 업무 화면 진입, 동적 모듈 열거, 매개변수 행렬과 여러 샘플, 권한 트리 보완 여부, 병합 결과, 자산 저장과 한계 보고를 확인하세요. 수행하지 않은 단계가 있으면 완전한 조사라고 주장하지 않습니다.

## I. 권한 트리 복원

auth 모듈에서 routeMap/routeLink와 userRouteAuth를 추출하고 권한 트리·stub을 만듭니다. permissions 수, flat codes/tree의 일치, route_map의 모든 link가 routes에 포함되는지 확인합니다. 갱신 후 runtime을 다시 실행하고 bootstrap 외 모듈 API가 늘었는지 비교하세요. 실패하면 stub 외부 형식, 코드 필드, hash/history와 대기 시간을 점검합니다.

```bash
grep -l 'userRouteAuth' js/*.js
grep -l 'routeMap\|routeLink' js/*.js
grep -rhoaE 'getResultTree|role_permissions|permissions/all' js | head
```

```bash
python3 scripts/extract_route_map.py recon/js recon/
```

```bash
grep -rhoaE '([A-Z_][A-Z0-9_]*):\{name:"[^"]*",link:"/[^"]+"\}' js | head -20
```

```bash
python3 scripts/build_perm_tree.py recon/js recon/ --config recon/config.json
```

```bash
wc -l recon/perm_codes_all.txt
python3 -c "import json; m=json.load(open('recon/route_map.json')); r=set(json.load(open('recon/config.json'))['routes']); print('missing', [v['link'] for v in m.values() if v['link'] not in r])"
```

```bash
node recon/runtime_harvest.js recon/config.json
```

## J. 매개변수 분석

알려진 path 주변의 요청 조립 객체와 wrapper를 찾고 query/json/form/graphql/encrypted 전송을 구분하세요. UI 필드 → submit → transform → payload, 검증 규칙과 선택 항목, interceptor 주입을 추적합니다. 필수/선택/조건부 판단은 여러 샘플의 diff와 검증 근거를 결합하세요. 신뢰도는 여러 runtime 샘플과 검증 근거가 있으면 높음, 단일 runtime 또는 명확한 정적 조립은 중간, 이름·주변 코드 추정은 낮음, UI 미관측은 미발동으로 표시합니다. 단일 샘플에서 생략된 필드를 선택 사항이라고 단정하지 마세요.

```bash
grep -n '"/api/user/list"' js/*.js
grep -rhoaE '.{0,120}("/api[^"]+").{0,200}' js | head
grep -rhoaE '(params|data|body|payload)\s*:\s*\{' js | head
grep -rhoaE '(get|post|put|delete|patch)\([^,]+,\s*\{' js | head
```

```bash
grep -rhoaE '(axios|request)\.(get|post|put|delete|patch)\(' js | head
grep -rhoaE 'interceptors\.(request|response)' js | head

# GraphQL
grep -rhoaE '(query|mutation)\s+\w+|gql`|graphql\(' js | head
grep -rhoaE '\$[a-zA-Z_]+\s*:\s*(Int|String|Boolean|\[)' js | head

# FormData / multipart
grep -rhoaE 'FormData|\.append\(' js | head

grep -rhoaE 'path:\s*"/[^"]*:[^"]+"' js | head
grep -rhoaE 'useParams|route\.params|\$route\.params' js | head
```

```bash
grep -rhoaE '(required|message|pattern|enum|validator)\s*:' js | head
grep -rhoaE 'yup\.|zod\.|async-validator|Form\.Item|a-form-item|el-form-item' js | head
grep -rhoaE 'rules\s*:\s*\[|name:\s*["\'][a-zA-Z_]+["\']' js | head
grep -rhoaE 'label.*value|options\s*:\s*\[' js | head
```

```bash
grep -rhoaE 'onFinish|handleSubmit|getFieldsValue|validateFields' js | head
grep -rhoaE '(pick|omit|transform|dayjs|moment)\(' js | head
```

```bash
grep -rhoaE 'encrypt|decrypt|sign|CryptoJS|sm2|sm3|sm4|RSA|AES' js | head
```
