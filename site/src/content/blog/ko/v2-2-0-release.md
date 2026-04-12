---
title: "Tetora v2.2 — 기본 안전 설계, 멀티 테넌트 Dispatch"
lang: ko
date: "2026-03-30"
tag: release
readTime: "약 6분"
excerpt: "DangerousOpsConfig가 agent 실행 전에 위험한 명령을 차단합니다. 멀티 테넌트 --client 플래그, Worktree 장애 보호, History CLI 실패 분석——v2.2은 agent dispatch를 프로덕션 수준으로 끌어올립니다."
description: "Tetora v2.2은 DangerousOpsConfig 명령 차단, 멀티 테넌트 dispatch 격리, Worktree 장애 보호, Self-liveness watchdog, History CLI 진단 도구를 추가했습니다. 세 개의 패치 릴리즈에 걸쳐 30개 이상의 개선이 이루어졌습니다."
---

Tetora v2.2은 세 가지 측면에서 기준을 높입니다: **안전성, 실행 신뢰성, 멀티 테넌트 격리**. v2.2.0부터 v2.2.2까지 세 번의 패치 릴리즈에 걸친 30개 이상의 개선으로 멀티 agent 병렬 dispatch가 더욱 견고해졌으며, 엔터프라이즈급 배포를 위한 기반이 마련되었습니다.

> **한 줄 요약:** DangerousOpsConfig가 agent 실행 전에 파괴적인 명령을 차단합니다. Worktree 격리가 모든 태스크를 커버합니다. 새로운 History CLI로 실패 분석이 가능해졌습니다. `--client` 플래그로 멀티 테넌트 워크스페이스 격리를 구현합니다. Pipeline 개편으로 좀비 프로세스를 제거합니다. Self-liveness watchdog이 응답 불능 daemon을 자동으로 재시작합니다.

## 안전 최우선: DangerousOpsConfig

v2.2에서 가장 중요한 변경 사항은 새로운 기능이 아닙니다——가드레일입니다.

**DangerousOpsConfig**는 패턴 기반 명령 차단 엔진입니다. agent가 쉘 명령을 실행하기 전에, Tetora가 설정 가능한 차단 목록과 대조합니다. 매칭된 경우 명령은 실행 전에 거부되어 부작용도, 데이터 손실도 발생하지 않습니다.

기본 차단 패턴:
- `rm -rf` (및 변형)
- `DROP TABLE`, `DROP DATABASE`
- `git push --force`
- `find ~/` (광범위한 `$HOME` 스캔)

`config.json`에서 커스텀 allowlist를 설정할 수 있습니다:

```json
{
  "dangerousOps": {
    "enabled": true,
    "extraPatterns": ["truncate", "kubectl delete"],
    "allowlist": ["rm -rf ./dist"]
  }
}
```

agent `AddDirs`에서 `$HOME`을 차단하는 수정 사항과 결합하여, agent는 지시를 받더라도 홈 디렉터리 전체에 실수로 접근할 수 없게 되었습니다. prompt 수준뿐만 아니라 심층 방어를 구현합니다.

## 신뢰성: Pipeline 전면 개편

v2.2은 프로덕션 환경의 안정성을 높이기 위해 pipeline 실행 레이어를 전면 재작성했습니다:

- **비동기 `scanReviews` + 세마포어** — 병렬 review 스캔을 최대 3개로 제한하여 대량 review 시 CPU 급증 방지
- **Pipeline 헬스 체크 모니터** — 백그라운드에서 30분마다 실행하여 `doing` 상태에 멈춘 좀비 태스크를 자동 리셋
- **타임아웃 시 프로세스 그룹 종료** — 파이프라인 단계가 타임아웃되면 전체 프로세스 그룹을 종료하여 고아 프로세스 완전 제거
- **에스컬레이트된 review 자동 승인** — 4시간 이상 체류한 에스컬레이트 review를 자동 승인하여 무한 차단 방지

Workspace Git 레이어도 강화되었습니다: `index.lock` 재시도에 지수 백오프 추가, `wsGitMu` 직렬화 잠금, stale lock 임계값을 1시간에서 30초로 단축.

## Self-Liveness Watchdog

프로덕션 배포에 자동 크래시 복구 기능이 추가되었습니다. 새로운 self-liveness watchdog이 Tetora daemon의 하트비트를 모니터링하고, 프로세스가 응답하지 않으면 supervisor 관리 재시작을 트리거합니다.

새벽 3시에 멈춰버린 daemon을 SSH로 수동 재시작할 필요가 없어졌습니다.

## 멀티 테넌트 Dispatch: `--client` 플래그

멀티 테넌트 지원이 공식적으로 추가되었습니다. 새로운 `--client` 플래그로 클라이언트별 dispatch 출력을 완전히 격리할 수 있습니다:

```bash
tetora dispatch --client acme "주간 보고서 workflow 실행"
tetora dispatch --client initech "PR #42 코드 리뷰"
```

각 클라이언트는 독립된 출력 경로를 가지며, 서로 다른 클라이언트의 태스크 출력이 혼재되지 않습니다. Team Builder CLI와 결합하면 단일 Tetora 인스턴스에서 멀티 클라이언트 agent 설정을 관리할 수 있습니다.

## Worktree 장애 보호

기존에는 태스크가 중간에 실패하면 worktree 정리 시 커밋되지 않은 변경 사항이 삭제되었습니다. v2.2부터는 커밋이나 로컬 변경 사항이 있는 실패/취소된 태스크가 삭제되지 않고 `partial-done` 상태로 보존됩니다.

이것이 의미하는 바:
1. 진행 중인 작업이 조용히 사라지는 일이 없어짐
2. agent가 어느 단계에서 실패했는지 정확히 확인 가능
3. 수동 복구가 간단해짐——브랜치가 완전한 상태로 남아 있음

## History CLI: 실패 분석

세 개의 새로운 `tetora history` 서브 커맨드로 agent 실행 실패를 진단할 수 있습니다:

```bash
tetora history fails              # 최근 실패한 태스크와 에러 요약 목록 표시
tetora history streak             # 각 agent의 연승/연패 기록 표시
tetora history trace <task-id>    # 특정 태스크의 전체 실행 추적
```

agent가 반복적으로 실패할 때, `history fails`와 `trace`로 원시 로그를 뒤지지 않고도 근본 원인을 파악할 수 있습니다.

## 취소 버튼 (v2.2.1)

Dashboard에서 직접 실행 중인 태스크를 취소하는 기능이 추가되었습니다:

- **Task Detail 모달** — 태스크가 `doing` 상태일 때 노란색 「Cancel」 버튼 표시
- **Workflow 진행 패널** — 「View Full Run」 옆에 「Cancel Run」 버튼 추가

태스크가 완료되거나 `doing` 상태가 아닐 때 버튼은 자동으로 숨겨집니다.

## Provider Preset UI

Dashboard Settings에 Provider Preset UI가 추가되었습니다:

- **Custom `baseUrl`** 입력 필드 (자체 호스팅 또는 프록시 엔드포인트 지원)
- **Anthropic 네이티브 provider 타입** — 올바른 헤더 형식의 `x-api-key` 인증 사용
- **연결 테스트 엔드포인트** — 태스크를 디스패치하기 전에 provider 설정 검증 가능

## 보안 수정

v2.2은 내부 감사에서 발견된 두 가지 보안 문제를 수정했습니다:

- **SSRF 수정** — `/api/provider-test` 엔드포인트 강화. 사용자 제공 URL은 아웃바운드 요청 전에 검증
- **XSS 수정** — Provider preset UI 입력 필드를 새니타이징하여 Dashboard 뷰에서의 크로스 사이트 스크립팅 방지

## v2.2.2로 업그레이드

```bash
tetora upgrade
```

단일 바이너리. 외부 의존성 없음. macOS / Linux / Windows 지원.

[GitHub에서 전체 Changelog 보기](https://github.com/TakumaLee/Tetora/releases/tag/v2.2.2)
