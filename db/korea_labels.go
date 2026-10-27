package db

// Exact legacy labels are retained solely for migration. Rules keep their IDs,
// patterns, actions, priorities, enabled state and any user-edited text.
var koreanAssetRuleLabels = [][2]string{
	{"[内置] 政府网站 (.gov)", "[기본] 정부 사이트 (.gov)"},
	{"[内置] 政府网站 (.gov.cn)", "[기본] 정부 사이트 (.gov.cn)"},
	{"[内置] 教育网站 (.edu)", "[기본] 교육 사이트 (.edu)"},
	{"[内置] 教育网站 (.edu.cn)", "[기본] 교육 사이트 (.edu.cn)"},
}
var koreanRuleLabels = [][4]string{
	{"[内置] 递归强制删除 rm -rf", "[기본] 재귀적 강제 삭제 rm -rf", "禁止执行递归强制删除（rm -rf / rm --recursive），可能永久损坏系统或靶机环境", "재귀적 강제 삭제(rm -rf / rm --recursive)는 시스템이나 대상 환경을 영구 손상할 수 있어 금지됩니다"},
	{"[内置] 删除系统关键目录", "[기본] 시스템 핵심 디렉터리 삭제", "禁止删除系统关键路径", "시스템 핵심 경로 삭제는 금지됩니다"},
	{"[内置] 磁盘格式化 mkfs", "[기본] 디스크 포맷 mkfs", "禁止格式化磁盘（mkfs）", "디스크 포맷(mkfs)은 금지됩니다"},
	{"[内置] 覆写磁盘设备 dd", "[기본] 디스크 장치 덮어쓰기 dd", "禁止使用 dd 覆写磁盘设备", "dd를 사용한 디스크 장치 덮어쓰기는 금지됩니다"},
	{"[内置] Fork 炸弹", "[기본] Fork 폭탄", "禁止执行 Fork 炸弹", "Fork 폭탄 실행은 금지됩니다"},
	{"[内置] 关机 / 重启", "[기본] 시스템 종료 / 재시작", "禁止执行关机或重启命令", "시스템 종료 또는 재시작 명령은 금지됩니다"},
	{"[内置] 杀死全部进程", "[기본] 전체 프로세스 종료", "禁止 kill -9 -1 或 killall -9（杀死所有进程）", "kill -9 -1 또는 killall -9(전체 프로세스 종료)는 금지됩니다"},
	{"[内置] 磁盘擦除 shred / wipe", "[기본] 디스크 지우기 shred / wipe", "禁止对磁盘设备执行 shred/wipe 擦除", "디스크 장치에 대한 shred/wipe 지우기는 금지됩니다"},
	{"[内置] 清空防火墙规则", "[기본] 방화벽 규칙 비우기", "禁止清空防火墙规则（iptables -F / nft flush）", "방화벽 규칙 비우기(iptables -F / nft flush)는 금지됩니다"},
	{"[内置] SQL DROP DATABASE / TABLE / SCHEMA", "[기본] SQL DROP DATABASE / TABLE / SCHEMA", "禁止执行 DROP 操作，可能不可逆地销毁数据库对象", "DROP은 데이터베이스 객체를 복구할 수 없게 삭제할 수 있어 금지됩니다"},
	{"[内置] SQL TRUNCATE", "[기본] SQL TRUNCATE", "禁止执行 TRUNCATE，可能清空数据表所有数据", "TRUNCATE는 테이블의 모든 데이터를 지울 수 있어 금지됩니다"},
	{"[内置] MongoDB drop / dropDatabase", "[기본] MongoDB drop / dropDatabase", "禁止执行 MongoDB drop 操作", "MongoDB drop 실행은 금지됩니다"},
	{"[内置] Redis FLUSHALL / FLUSHDB", "[기본] Redis FLUSHALL / FLUSHDB", "禁止执行 Redis FLUSHALL / FLUSHDB，可能清空全部缓存数据", "Redis FLUSHALL / FLUSHDB는 모든 캐시 데이터를 지울 수 있어 금지됩니다"},
	{"[内置] curl / wget 发送 DELETE 请求", "[기본] curl / wget DELETE 요청", "禁止通过 curl/wget 发送 HTTP DELETE 请求，可能删除目标系统数据", "curl/wget의 HTTP DELETE 요청은 대상 시스템의 데이터를 삭제할 수 있어 금지됩니다"},
	{"[内置] Python HTTP 客户端 DELETE（requests/httpx/aiohttp）", "[기본] Python HTTP 클라이언트 DELETE (requests/httpx/aiohttp)", "禁止使用 Python HTTP 客户端发送 DELETE 请求", "Python HTTP 클라이언트의 DELETE 요청은 금지됩니다"},
	{"[内置] 脚本中声明 HTTP DELETE 方法（JS/通用）", "[기본] 스크립트의 HTTP DELETE 메서드 (JS/일반)", "禁止在脚本中声明并发送 HTTP DELETE 请求", "스크립트에서 HTTP DELETE 요청을 선언하고 전송하는 것은 금지됩니다"},
	{"[内置] 批量清空 / 清除接口路径", "[기본] 일괄 비우기 / 지우기 API 경로", "禁止调用批量清空或销毁类接口（/clear /wipe /flush /purge 等）", "일괄 비우기 또는 삭제 API(/clear /wipe /flush /purge 등) 호출은 금지됩니다"},
	{"[内置] 破坏性系统命令", "[기본] 파괴적인 시스템 명령", "破坏性命令被拒绝（rm -rf / / mkfs / dd / fork bomb / 关机重启 / 覆写磁盘设备）", "파괴적인 명령이 거부되었습니다(rm -rf / / mkfs / dd / fork bomb / 시스템 종료·재시작 / 디스크 장치 덮어쓰기)"},
	{"[内置] 数据外泄管道", "[기본] 데이터 유출 파이프", "疑似数据外泄管道被拒绝（命令输出经 curl/wget/nc 外传）", "데이터 유출로 의심되는 파이프가 거부되었습니다(curl/wget/nc로 명령 출력 외부 전송)"},
	{"[内置] 删除类接口路径", "[기본] 삭제 API 경로", "禁止调用删除类接口（/delete /remove /unlink /erase 等），不论使用哪种 HTTP 方法——多数应用的删除接口用 GET/POST 就能触发，同样会真实删除目标数据", "HTTP 메서드와 관계없이 삭제 API(/delete /remove /unlink /erase 등) 호출은 금지됩니다. 많은 앱의 삭제 API는 GET/POST로도 실제 대상 데이터를 삭제합니다"},
}

func (d *DB) upgradeKoreanRuleLabels() error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Serialize startup migrations from multiple server instances.
	if _, err = tx.Exec(`SELECT pg_advisory_xact_lock(7337741011)`); err != nil {
		return err
	}
	var done bool
	if err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM settings WHERE key='korea_rule_labels_v1' AND value='true')`).Scan(&done); err != nil {
		return err
	}
	if done {
		return nil
	}
	if _, err = tx.Exec(`UPDATE agents SET name=$2 WHERE key='retester' AND name=$1`, "漏洞复测", "취약점 재검증"); err != nil {
		return err
	}
	if _, err = tx.Exec(`UPDATE agents SET description=$2 WHERE key='retester' AND description=$1`, "从漏洞详情手动启动，读取原证据并保存独立复测结论。", "취약점 상세에서 시작하여 원래 증거를 읽고 독립적인 재검증 결과를 저장합니다."); err != nil {
		return err
	}
	for _, r := range koreanAssetRuleLabels {
		if _, err = tx.Exec(`UPDATE asset_intercept_rules SET note=$2 WHERE builtin AND note=$1`, r[0], r[1]); err != nil {
			return err
		}
	}
	for _, r := range koreanRuleLabels {
		if _, err = tx.Exec(`UPDATE intercept_rules SET message=$3 WHERE name IN ($1,$2) AND message=$4`, r[0], r[1], r[3], r[2]); err != nil {
			return err
		}
		if _, err = tx.Exec(`UPDATE intercept_rules SET name=$2 WHERE name=$1`, r[0], r[1]); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(`INSERT INTO settings(key,value) VALUES('korea_rule_labels_v1','true') ON CONFLICT(key) DO UPDATE SET value=EXCLUDED.value`); err != nil {
		return err
	}
	return tx.Commit()
}
