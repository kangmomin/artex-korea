---
name: scopesentry-mcp
description: 이미 연결된 ScopeSentry MCP로 프로젝트, 스캔 작업, 템플릿, 자산과 노드를 관리합니다.
---

# ScopeSentry MCP 사용 안내

배포된 ScopeSentry 인스턴스와 허가된 작업 범위를 사용합니다. MCP 도구의 실제 schema가 매개변수 계약의 기준입니다. 원문은 [SKILL.zh.md](SKILL.zh.md)에 보존되어 있습니다.

## 연결

Web은 `http://<host>`, MCP는 실제 프록시 구성을 반영한 `http://<host>/mcp`입니다. Web의 API Key 관리에서 키를 만들고 한 번만 표시되는 `ssk_...`를 보관합니다. MCP 클라이언트의 URL과 `X-API-Key` 또는 `Authorization: Bearer` 헤더에 설정하고 도구 목록을 확인하세요. 자격 증명은 문서나 보고서에 기록하지 마세요.

```json
{"mcpServers":{"scopesentry":{"url":"http://<host>:8082/mcp","headers":{"X-API-Key":"ssk_<key>"}}}}
```

## 도구

| 도구 | 용도 |
|---|---|
| list_projects / list_projects_data | 프로젝트 트리·페이지 목록, ID 조회 |
| get_project / create_project | 프로젝트 조회·생성 |
| list_tasks / get_task | 스캔 작업 목록·상세 |
| list_scan_templates / get_scan_template | 템플릿 목록·상세 |
| list_plugin_modules / list_plugins | 모듈명, 플러그인 hash와 기본 parameter |
| create_scan_template / create_scan_task | 템플릿·작업 생성 |
| list_assets / count_assets | 자산 목록·총수 |
| get_asset_detail / add_asset_tag | 자산·취약점 상세, 태그 추가 |
| list_nodes | 스캔 노드 목록 |

총수는 count_assets로 조회하세요. 수를 세려고 list_assets의 모든 페이지를 읽지 마세요. 두 도구의 search/filter 문법은 같습니다.

## 프로젝트와 작업

프로젝트가 지정되면 list_projects 또는 list_projects_data에서 ObjectID를 찾아 `filter.project`로 범위를 좁히세요. 표시 이름을 ID 자리에 넣지 마세요. 프로젝트 맥락이 없으면 임의로 선택하지 않습니다.

작업 생성 시 list_nodes의 온라인 **노드 이름**, 템플릿 **ObjectID**를 사용합니다. name과 node는 필수입니다. 템플릿 이름을 template 값으로 전달하지 마세요.

| targetSource | 필수 및 선택 값 |
|---|---|
| general | target |
| project | project: 프로젝트 ObjectID 배열 |
| asset / RootDomain / subdomain / UrlScan | search; 선택 project, filter, targetNumber |
| subdomainSource 등 *Source | targetTp=search는 search, targetTp=select는 targetIds |

루트 도메인의 전체 수집은 두 단계로 나눕니다. 먼저 general 입력과 SubdomainScan/SubdomainSecurity만 활성화한 템플릿으로 서브도메인을 수집하고 get_task로 완료를 확인하세요. 다음은 subdomain 소스와 `task=="<첫 작업 이름>"` 검색, 후속 모듈 템플릿을 사용합니다. 서브도메인을 독립 대상으로 배분하여 노드 편중을 줄입니다. 한 번에 전체 파이프라인을 돌리면 루트 도메인을 맡은 노드에 후속 작업이 몰릴 수 있습니다.

템플릿 생성은 list_plugin_modules → list_plugins → create_scan_template 순서이며 modules에는 모듈명별 플러그인 hash 배열을 지정합니다.

## 자산 조회

asset_type의 대소문자를 유지하세요:

`asset`, `RootDomain`, `subdomain`, `app`, `mp`, `UrlScan`, `SensitiveResult`, `DirScanResult`, `crawler`, `vulnerability`, `PageMonitoring`, `IPAsset`, `SubdomainTakerResult`.

별칭은 web→asset, vuln→vulnerability, ip→IPAsset, url→UrlScan입니다. pageIndex/pageSize 기본값은 1/20입니다. search와 filter를 함께 쓸 수 있고 SensitiveResult의 sid는 민감 규칙 이름입니다. count_assets는 같은 조건으로 `{"total":N}`을 반환합니다.

### 검색식

SQL이 아닌 전용 DSL입니다. `=`는 정규식 부분 일치, `==`는 완전 일치, `!=`는 제외, `&&`는 AND, `||`는 OR입니다. 인덱스 필드는 완전 일치 또는 `^`로 시작하는 접두사 검색을 우선하고 큰 데이터에서 무제한 부분 일치를 피하세요.

- `domain==www.example.com && port==443`
- `domain=^example.com`
- `task=="조사 작업"`
- `level==high` (vulnerability)
- `statuscode==200` (DirScanResult)

공통 검색 필드는 tag, task(작업 이름), rootDomain입니다. **project는 search에 넣지 말고 filter.project를 사용하세요.**

| asset_type           | 필드                                                                                  |
| -------------------- | ----------------------------------------------------------------------------------- |
| asset                | domain, ip, port, service, app, title, statuscode, icon, banner, type, body, header |
| RootDomain           | domain, icp, company                                                                |
| subdomain            | domain, ip, type, value                                                             |
| app                  | name, icp, company, category, description, url, apk                                 |
| mp                   | name, icp, company, category, description, url                                      |
| UrlScan              | url, input, source, resultId, type                                                  |
| SensitiveResult      | url, sname, body, info, md5                                                         |
| DirScanResult        | url, statuscode, redirect, length                                                   |
| vulnerability        | url, vulname, matched, request, response, level                                     |
| crawler              | url, method, body, resultId                                                         |
| PageMonitoring       | url, hash, diff, response                                                           |
| IPAsset              | ip, domain, port, service, webServer, app                                           |
| SubdomainTakerResult | domain, value, type, response                                                       |

### 정확한 필터

같은 키의 여러 값은 OR, 다른 키는 AND입니다. project는 ObjectID, task는 작업 이름입니다. port/service/app/icon/statuscode/type/tags 등은 아래 지원표를 따릅니다. level은 critical/high/medium/low/info입니다. SensitiveResult의 color/sname은 규칙 색상/이름이고 status는 처리 상태입니다. UrlScan/DirScanResult의 HTTP 코드는 filter.status를 사용합니다.

| asset_type                            | filter key                                                      |
| ------------------------------------- | --------------------------------------------------------------- |
| asset                                 | project, port, service, app, icon, statuscode, type, task, tags |
| RootDomain                            | project, tags                                                   |
| subdomain                             | project, type, task, tags                                       |
| app / mp                              | project, tags                                                   |
| UrlScan                               | status, tags                                                    |
| DirScanResult                         | status, tags                                                    |
| SensitiveResult                       | status, color, sname, tags                                      |
| crawler                               | project, task, tags                                             |
| vulnerability                         | project, level, status, task, tags                              |
| PageMonitoring / SubdomainTakerResult | tags                                                            |
| IPAsset                               | project, port, service, app                                     |

```json
{"asset_type":"asset","search":"domain=^example.com && port==443","filter":{"project":["<projectObjectID>"]},"pageIndex":1,"pageSize":10}
```

sort는 UrlScan과 DirScanResult에서만 `{"length":"ascending"}`을 지원하고 다른 유형은 무시하여 기본 시간순으로 정렬합니다.

## 스캔 모듈 이름

`TargetHandler`, `SubdomainScan`, `SubdomainSecurity`, `PortScanPreparation`, `PortScan`, `PortFingerprint`, `AssetMapping`, `AssetHandle`, `URLScan`, `WebCrawler`, `URLSecurity`, `DirScan`, `VulnerabilityScan`, `PassiveScan`.

## 문제 해결

- 도구가 없으면 MCP URL, API Key, 서버 실행과 도구 활성화를 확인합니다.
- 401/403이면 유효한 API Key와 권한을 확인합니다.
- 자산이 없으면 project에 표시 이름 대신 ObjectID가 들어갔는지, search에 project를 쓰지 않았는지 확인합니다.
- 템플릿/작업 생성이 실패하면 template ObjectID와 온라인 node 이름을 확인합니다.
- 조회가 느리면 지원되는 project 필터로 범위를 줄이고 `==`/`^`를 사용하며 pageSize를 줄입니다.
