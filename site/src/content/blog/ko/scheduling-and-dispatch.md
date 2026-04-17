---
title: "Tetora의 스케줄링 시스템 — 간단한 Cron부터 복잡한 워크플로우까지"
lang: ko
date: "2026-04-11"
tag: explainer
readTime: "약 6분"
excerpt: "Tetora가 하나의 cron 표현식을 완전 자동화된 Human Gate 워크플로우로 바꾸는 방법과 스케줄링 스택의 각 요소를 언제 사용해야 하는지 설명한다."
description: "Tetora 스케줄링 시스템 완전 가이드: cron 작업, dispatch 큐, Human Gate 승인 체크포인트, 복잡한 다단계 자동화를 위한 Workflow DAG."
---

대부분의 AI 에이전트 도구는 대화형이다. 프롬프트를 입력하면 에이전트가 응답한다. 임시 작업에는 충분하다. 하지만 시간을 가장 많이 절약하는 자동화는 당신 없이도 실행되는 자동화다.

Tetora에는 완전한 스케줄링 스택이 내장되어 있다. 이 글에서는 각 레이어를 설명하고, 언제 사용해야 하는지, 그리고 개별 구성 요소보다 훨씬 강력한 것으로 어떻게 조합되는지 살펴본다.

## 레이어 1: Cron 스케줄러

가장 단순한 스케줄링 기본 단위는 cron 작업이다. 실행 시간과 dispatch할 내용을 지정한다:

```bash
tetora job add --cron "0 21 * * *" "Run nightly accounting check"
tetora job add --cron "0 9 * * 1" "Weekly team standup summary"
```

표준 cron 문법 — 분, 시간, 일, 월, 요일의 다섯 필드다. cron을 사용해본 적이 있다면 새로 배울 것이 없다. 없더라도 온라인에 cron 표현식 생성기가 많다. Tetora는 유효한 cron 문자열이면 무엇이든 받는다.

예약된 작업 확인:

```bash
tetora job list
```

작업 제거:

```bash
tetora job remove <job-id>
```

cron 작업이 실행되면 dispatch 큐에 태스크를 생성하고 즉시 반환한다. cron 스케줄러는 태스크 완료를 기다리지 않는다. 태스크가 제때 생성되는 것만 보장한다.

## 레이어 2: Dispatch 큐

Dispatch 큐는 실행 레이어다. 태스크는 다양한 경로로 큐에 들어온다. cron 작업, 수동 `tetora dispatch` 호출, 워크플로우 단계, webhook 트리거. 그리고 에이전트가 이를 수행한다.

큐의 동시 실행 수는 설정 가능하다:

```json
{
  "dispatch": {
    "maxSlots": 4,
    "slotPressureThreshold": 0.8
  }
}
```

`maxSlots`는 동시에 실행되는 태스크 수를 제어한다. 모든 슬롯이 가득 찬 상태에서 새 태스크가 들어오면, 강제로 시작해 리소스를 경쟁하는 대신 큐에서 대기한다. `slotPressureThreshold`는 추가 완충재를 더한다. 슬롯 사용률이 이 비율을 초과하면 기술적으로 아직 자리가 있어도 새 태스크는 큐에 들어간다.

대부분의 개인 환경에서는 `maxSlots: 2`나 `3`이 적당하다. 병렬 태스크를 실행하면서도 로컬 리소스를 과부하시키지 않는 균형이다.

큐 상태는 언제든 확인할 수 있다:

```bash
tetora status
```

실행 이력 검토:

```bash
tetora history fails          # 최근 실패 표시
tetora history trace <task-id>  # 특정 태스크의 전체 추적
```

## 레이어 3: Human Gate

완전 자동화 파이프라인에 넣어서는 안 되는 태스크가 있다. 복잡하기 때문이 아니라 결과가 중대하기 때문이다. 되돌리기 어려운 작업, 실제 외부 시스템에 영향을 미치는 액션, 에이전트가 단독으로 판단해서는 안 되는 의사결정이 그것이다.

Human Gate는 워크플로우 단계에 승인 체크포인트를 추가한다. 에이전트가 해당 단계에 도달하면 일시 중지하고, 알림을 보내고, 명시적인 승인을 받을 때까지 대기한다.

```json
{
  "humanGate": {
    "enabled": true,
    "timeoutHours": 4,
    "notifyDiscord": true
  }
}
```

`timeoutHours`는 에이전트가 에스컬레이션하거나 단계를 포기하기 전에 대기하는 시간을 제어한다. `notifyDiscord`는 승인이 필요할 때 설정된 Discord 채널에 메시지를 보낸다. 자리를 비운 동안 실행되는 워크플로우에 유용하다.

워크플로우 YAML에서는 `humanGate: true`로 해당 단계를 표시한다:

```yaml
- id: review-uncertain
  humanGate: true
  run: "Flag transactions with confidence < 0.8 for human review"
  depends: [classify]
```

에이전트는 이 단계에 도달하면 신뢰도가 낮은 거래를 검토를 위해 제시하고 대기한다. CLI나 Discord로 승인하면 다음 단계로 진행한다. 거부하면 워크플로우가 그 자리에서 중단되고 결과가 기록된다.

Human Gate는 자동화를 덜 유용하게 만들지 않는다. 완전한 자율성이 부적절한 상황에서도 안전하게 사용할 수 있게 만든다.

## 레이어 4: Workflow DAG

각 태스크가 독립적일 때는 단순한 cron 작업으로 충분하다. 단계 간 의존 관계가 있는 다단계 프로세스에는 Workflow DAG를 사용하면 전체 파이프라인을 선언적으로 정의할 수 있다.

실용적인 예시를 보자. 세무사 하타케야마 켄토가 사용하는 패턴을 기반으로 한 매일 밤 실행되는 회계 워크플로우다:

```yaml
name: nightly-accounting
steps:
  - id: fetch-transactions
    run: "Fetch today's unprocessed transactions from the freee API"
  - id: classify
    run: "Classify each transaction by account category"
    depends: [fetch-transactions]
  - id: review-uncertain
    humanGate: true
    run: "Flag transactions with confidence < 0.8 for human review"
    depends: [classify]
  - id: post-entries
    run: "Post approved entries to freee"
    depends: [review-uncertain]
```

실행은 DAG를 따라 흐른다. `fetch-transactions`가 먼저 실행되고, 그 다음 `classify`(가져오기 완료에 의존), 그 다음 `review-uncertain`(사람의 승인을 위해 일시 중지), 마지막으로 `post-entries`(승인 후에만 실행)의 순이다.

`fetch-transactions`가 실패하면 다운스트림의 어떤 것도 실행되지 않는다. `review-uncertain`이 승인 없이 시간 초과되면 `post-entries`는 실행되지 않는다. DAG 구조가 실패 모드를 명확하고 추적 가능하게 만든다.

`depends` 필드가 없는 단계는 동시에 준비되면 병렬로 실행될 수 있다. 병합 단계 전에 여러 독립적인 소스에서 데이터를 가져오는 워크플로우에서는 가져오기가 동시에 실행되어 전체 실행 시간이 줄어든다.

### 조건 라우팅

단계에는 `condition` 필드를 포함할 수 있다. 조건이 참일 때만 단계가 실행된다:

```yaml
- id: send-alert
  run: "Send Slack alert about anomalous transactions"
  condition: "{{classify.anomaly_count}} > 0"
  depends: [classify]
```

분류 단계에서 이상이 발견되지 않으면 알림 단계는 완전히 건너뛴다. 별도의 트리거 메커니즘 없이 파이프라인이 데이터에 따라 동작을 조정한다.

## 조합하면

이 레이어들은 깔끔하게 조합된다:

- **Cron**이 매일 밤 21:00에 작업을 실행한다
- 그 작업이 **Workflow** 태스크를 dispatch한다
- 워크플로우가 **dispatch 큐**를 통해 여러 자동화 단계를 실행한다
- 중요한 쓰기 단계에서 **Human Gate**가 승인을 위해 일시 중지한다
- 확인 후 나머지 워크플로우가 완료된다

결과는 스케줄에 따라 안정적으로 실행되고, 오류를 예측 가능하게 처리하고, 가능한 곳에서 병렬화하고, 자동화해서는 안 되는 판단을 사람에게 에스컬레이션하는 파이프라인이다.

이것이 바로 하타케야마가 수개월에 걸쳐 수동으로 구축한 설정이다. Tetora는 이를 커스텀 인프라 없이 사용할 수 있는 조합 가능한 시스템으로 제공한다.

## 설계 철학

Tetora 스케줄링 시스템의 목표는 올바른 제어 수준을 올바른 액션에 매핑하는 것이다.

API에서 데이터 가져오기? 완전 자동화. 키워드로 거래 분류? 완전 자동화. 고객의 재무 기록에 영향을 미치는 분개 전기? Human Gate. 고객에게 보고서 전송? Human Gate.

어떤 액션이 어느 범주에 속하는지에 대한 판단은 당신의 몫이다. 비즈니스 맥락과 위험 허용 범위를 이해하는 사람. Tetora는 그 판단을 한 번 인코딩하면 매번 실행할 때 일관되게 적용되는 메커니즘을 제공한다.

**일상적인 것은 자동화하고, 판단이 필요한 것은 에스컬레이션하라.** 스케줄링 스택은 그 원칙을 실제로 작동하게 만드는 방법이다.
