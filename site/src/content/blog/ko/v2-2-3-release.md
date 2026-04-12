---
title: "Tetora v2.2.3–v2.2.4 — 모델 피커, TTS, Human Gate"
lang: ko
date: "2026-04-04"
tag: release
readTime: "약 5분"
excerpt: "인터랙티브 모델 전환기, VibeVoice TTS, Human Gate 개선, Skill AllowedTools, Discord !model 커맨드."
description: "Tetora v2.2.3은 인터랙티브 모델 전환(Discord + Dashboard), VibeVoice TTS, Human Gate retry/cancel/알림, Skill AllowedTools, 세션 히스토리에서의 learned skill 자동 추출을 추가합니다."
---

v2.2.3과 v2.2.4가 함께 출시되었습니다. Discord와 Dashboard에서의 인터랙티브 모델 전환, VibeVoice를 통한 로컬 및 클라우드 TTS, 더욱 강력해진 Human Gate, 스킬별 도구 제한, 세션 히스토리에서의 자동 스킬 추출이 포함되었습니다. v2.2.4는 버그 수정과 인프라 강화를 이어갑니다.

> **한 줄 요약:** `!model pick`으로 Discord에서 인터랙티브하게 provider와 모델을 전환할 수 있습니다. VibeVoice로 로컬 TTS와 클라우드 폴백이 제공됩니다. Human Gate가 retry, cancel, Discord 알림을 지원합니다. `allowedTools`로 Skill이 호출할 수 있는 도구를 제한합니다. Learned skill은 세션 히스토리에서 자동으로 추출됩니다.

## 모델 전환

### Discord 커맨드

설정 파일을 수정하지 않고 추론 모델을 전환할 수 있습니다. Tetora가 활성화된 채널에서 세 가지 새로운 커맨드를 사용할 수 있습니다:

**`!model pick`** — 3단계 플로우의 인터랙티브 피커를 엽니다:

```
1단계: provider 선택  →  2단계: 모델 선택  →  3단계: 확인
```

각 단계는 번호가 매겨진 선택지가 포함된 Discord 메시지로 제공됩니다. 번호를 입력하면 다음 단계로 진행됩니다.

**`!local` / `!cloud`** — 모든 agent의 추론 모드를 한 번에 전환합니다. `!local`은 모든 agent를 설정된 로컬 provider(Ollama, LM Studio 등)로 전환합니다. `!cloud`는 클라우드 provider로 되돌립니다.

**`!mode`** — 현재 추론 설정의 요약을 출력합니다: 활성 provider, 모델, 글로벌 모드.

### Dashboard 모델 피커

이제 Dashboard가 agent 카드에 모델 설정을 직접 표시합니다:

- **Provider 바** — 각 agent 카드 상단에 활성 provider를 색상 코드 배지로 표시 (클라우드는 파란색, 로컬은 초록색)
- **모델 드롭다운** — agent 카드에서 클릭만으로 해당 agent의 모델을 전환 (Settings 이동 불필요)
- **글로벌 추론 모드 토글** — 헤더 바의 스위치 하나로 모든 agent를 Cloud와 Local 사이에서 일괄 전환

### Claude Provider 설정

`config.json`에 새로운 `claudeProvider` 필드가 추가되어 Tetora가 Claude 모델을 호출하는 방식을 제어합니다:

```json
{
  "claudeProvider": "claude-code"
}
```

- `"claude-code"` — Claude Code CLI를 통해 Claude를 호출합니다. 활성 Claude 구독이 있는 로컬 설치의 기본값.
- `"anthropic"` — `ANTHROPIC_API_KEY`를 사용해 Anthropic API를 직접 호출합니다. 헤드리스 환경이나 CI에서 실행할 때의 기본값.

설치 환경별로 설정할 수 있어 로컬 개발 머신과 원격 서버가 설정 충돌 없이 서로 다른 호출 경로를 사용할 수 있습니다.

## VibeVoice TTS

Tetora가 이제 말을 합니다. VibeVoice 통합으로 agent 응답에 TTS 출력이 추가되었으며, 2단계 폴백 체인을 제공합니다:

1. **로컬 VibeVoice** — 기기에서 실행, 모델 로드 후 제로 레이턴시, 완전한 프라이버시 보호
2. **fal.ai 클라우드 TTS** — 로컬 VibeVoice를 사용할 수 없거나 실패했을 때 자동으로 사용

`config.json`에서 설정합니다:

```json
{
  "tts": {
    "enabled": true,
    "provider": "vibevoice",
    "fallback": "fal"
  }
}
```

TTS는 기본적으로 비활성화되어 있습니다. 활성화하면 agent가 Discord 음성 채널과 Dashboard 모니터 뷰에서 응답을 읽어줍니다.

## Human Gate 개선

Human Gate——agent 실행을 일시 중지하고 사람의 승인을 요청하는 Tetora의 메커니즘——가 편의성 면에서 크게 향상되었습니다.

### Retry와 Cancel

리뷰어는 이제 수동 개입 없이 이전에 거부된 gate에 대해 조치를 취할 수 있습니다:

- **Retry API** — `POST /api/gate/:id/retry`로 gate를 리뷰 대기열에 재투입하고 상태를 `waiting`으로 리셋
- **Cancel API** — `POST /api/gate/:id/cancel`로 일시 중지된 태스크를 깔끔하게 종료
- 두 동작 모두 Dashboard의 Task Detail 모달에 기존 Approve/Reject 버튼과 함께 표시

### Discord 알림

Human Gate 이벤트가 설정된 알림 채널에 Discord 메시지를 트리거합니다:

- **Waiting** — gate가 열려 승인 대기 중일 때 리뷰어에게 알림
- **Timeout** — gate가 조치 없이 만료되면 영향받은 태스크를 포함하여 채널에 알림
- **Assignee 멘션** — gate에 담당 리뷰어가 있으면 알림에서 해당 사용자를 직접 `@mention`

### 통합 액션 필드

gate 이벤트 스키마가 승인 데이터를 두 개의 필드로 통합했습니다:

```json
{
  "action": "approve | reject | retry | cancel",
  "decision": "approved | rejected"
}
```

이것은 기존의 `approved`, `rejected`, `action` 필드가 혼재하던 구조를 대체합니다. 이전 필드는 한 릴리즈 사이클 동안 계속 읽을 수 있으며 그 이후 제거됩니다.

## Skill AllowedTools

Skill이 도구 제한 목록을 지원합니다. Skill 설정에 `allowedTools`를 설정하면 해당 Skill이 호출할 수 있는 MCP 도구를 제한할 수 있습니다:

```json
{
  "name": "freee-check",
  "allowedTools": ["mcp__freee__list_transactions", "mcp__freee__get_company"],
  "prompt": "Check unprocessed entries for all companies."
}
```

`allowedTools`가 설정되면 Skill은 샌드박스 context에서 실행되며, 쉘 커맨드, 파일 시스템 접근, 목록에 없는 MCP 도구를 포함한 다른 도구는 사용할 수 없게 됩니다. 이를 통해 Skill 레벨에서 최소 권한이 강제되고 감사 추적이 더욱 명확해집니다.

## Learned Skill 자동 추출

Tetora가 이제 세션 히스토리에서 재사용 가능한 패턴을 자동으로 식별하고 새로운 Skill로 제안합니다.

세션이 종료되면 백그라운드 프로세스가 대화를 스캔하여 반복되는 커맨드 시퀀스와 멀티스텝 패턴을 찾습니다. 후보 항목은 `SKILL.md`와 `metadata.json`과 함께 `skills/learned/`에 작성되고, 리뷰가 완료될 때까지 `approved: false`로 플래그가 표시됩니다.

CLI에서 제안된 skill을 검토합니다:

```bash
tetora skill list --pending      # 리뷰 대기 중인 제안 skill 표시
tetora skill approve <name>      # 활성 상태로 승격
tetora skill reject <name>       # 제안 삭제
```

승인된 skill은 즉시 슬래시 커맨드로 사용 가능합니다.

## v2.2.4 수정

v2.2.4는 안정화 릴리즈입니다. 주요 수정 사항:

- **i18n URL 중복 제거** — 생성된 URL에서 로케일 접두사가 이중으로 붙는 라우팅 버그를 수정했습니다 (예: `/en/en/blog/...` → `/en/blog/...`).
- **Skills cache RWMutex** — skills cache의 일반 mutex를 읽기-쓰기 mutex로 교체하여 읽기 집약적 워크로드의 처리량을 향상시켰습니다.
- **SEO 개선** — 모든 블로그와 문서 페이지에 `BreadcrumbList` 구조화 데이터와 올바른 `og:locale` 값을 추가했습니다.
- **리그레션 가드 테스트** — i18n URL 중복 제거 수정과 skills cache를 커버하는 통합 테스트를 추가하여 리그레션을 방지합니다.

## 업그레이드

```bash
tetora upgrade
```

단일 바이너리. 외부 의존성 없음. macOS / Linux / Windows 지원.

[GitHub에서 전체 Changelog 보기](https://github.com/TakumaLee/Tetora/releases/tag/v2.2.4)
