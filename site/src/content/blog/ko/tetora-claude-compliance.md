---
title: "Tetora가 공식 Claude API를 사용하는 이유"
lang: ko
date: "2026-04-10"
tag: explainer
readTime: "약 4분"
excerpt: "2026년 4월 Anthropic의 조치는 Claude 구독으로 허용되는 것을 명확히 했다. Tetora가 Claude와 통합하는 방법의 기술적 세부사항과 차단되지 않는 이유를 설명한다."
description: "Tetora는 공식 claude-code CLI 또는 직접 API 키를 통해 Claude와 통합한다. OAuth 구독 토큰은 사용하지 않는다. 기술적 세부사항과 안정성에 대한 의미를 설명한다."
---

2026년 2월 20일, Anthropic은 수많은 서드파티 Claude 도구 생태계에 영향을 미치는 이용 정책 업데이트를 발표했다. 핵심은 명확했다. **구독 OAuth 토큰은 서드파티 애플리케이션에서 사용할 수 없다**. 이 정책은 이전부터 같은 취지였지만, 이번 업데이트로 더욱 명시적이 되었다.

4월 4일, Anthropic은 OpenClaw에 이 정책을 집행하여 도구를 완전히 차단했다. 그 도구의 모든 사용자에게 서비스는 당일 중단되었다.

이 글에서는 Tetora가 Claude와 어떻게 통합되는지 정확히 설명하고, 왜 그 설계 선택이 이번 집행 조치의 영향을 받지 않는지 설명한다.

## 허용된 두 가지 통합 경로

Anthropic의 허용 이용 정책은 Claude를 사용하는 애플리케이션을 구축하기 위한 두 가지 방법을 명시한다:

1. **공식 `claude-code` CLI** — Anthropic이 배포하는 퍼스트파티 커맨드라인 인터페이스
2. **API 키를 사용한 Anthropic API** — console.anthropic.com에서 생성한 사용자 API 키로 인증하는 직접 HTTP 호출

두 경로 모두 안정적이고 명시적으로 지원되며, Anthropic의 표준 API 약관을 따른다. 두 경로 중 어느 것을 사용하든, Anthropic과의 결제 관계는 직접적이고 투명하다. 사용한 만큼 지불한다.

**허용되지 않는** 것은 Claude.ai 구독을 뒷받침하는 OAuth 토큰을 서드파티 도구에서 사용하는 것이다. 그 토큰은 claude.ai의 웹 세션을 인증하기 위한 것이며, 기술적으로 가능했더라도 외부 도구에서 사용하도록 허가된 적이 없다.

## Tetora의 설정 방법

Tetora의 통합 방식은 단일 설정 필드로 결정된다. `claudeProvider`다.

`claude-code` CLI가 설치된 사용자의 경우:

```json
{
  "claudeProvider": "claude-code"
}
```

Tetora는 `claude-code`를 서브프로세스로 호출한다. 터미널에서 직접 사용하는 것과 동일하다. CLI가 자체적으로 인증, 세션 처리, Anthropic 서버와의 통신을 담당하며, Tetora는 인증 자격 증명에 직접 접근하지 않는다.

직접 API 접근을 선호하는 사용자의 경우:

```json
{
  "claudeProvider": "anthropic"
}
```

이 모드에서 Tetora는 로컬 Tetora 설정(또는 환경 변수)에 저장된 API 키를 사용하여 Anthropic API를 직접 호출한다. 키는 Anthropic 콘솔에서 발급되며, 계정의 API 결제에 연결된다. 사용 한도 설정, 비용 모니터링, 키 교체 모두 Anthropic 자체 도구를 통해 직접 관리할 수 있다.

두 통합 경로 모두 구독 OAuth 토큰을 사용하지 않는다. 두 경로 모두 Anthropic의 결제나 접근 제어를 우회하지 않는다. 두 경로 모두 Anthropic이 지원하는 통합 패턴 그 자체다.

## 왜 안정성에 중요한가

Tetora 위에 자동화를 구축할 때, 두 가지 중요한 속성을 가진 기반 위에 구축하는 것이다.

**집행 리스크 없음.** Tetora는 Anthropic의 정책을 위반하는 어떤 행동도 하지 않는다. OpenClaw에 일어난 것과 같은 시나리오, 즉 정책 집행 조치로 하룻밤 사이에 워크플로우가 중단되는 상황은 존재하지 않는다.

**예측 가능한 비용.** API 키 사용량은 계량되어 Anthropic 계정에 청구된다. 하드 지출 한도를 설정하고, 임계값에서 알림을 받고, Anthropic 콘솔에서 요청별 비용 내역을 확인할 수 있다. 구독은 이런 세분화를 제공하지 않는다. 정액을 지불하고 사용량을 충당하기를 바랄 뿐이다. 스케줄에 따라 또는 이벤트에 반응하여 실행되는 자동화에는 종량제 결제가 실제 소비 패턴에 훨씬 잘 맞는다.

**이식성.** API 키는 노트북, 홈 서버, 원격 VM 어디에서 Tetora를 실행하든 동일하게 작동한다. 유지해야 할 계정 세션도, 인증을 유지해야 할 브라우저도, 주기적인 재인증 흐름도 없다. 키는 교체할 때까지 유효하다.

## 컴플라이언스에 대한 더 넓은 관점

컴플라이언스를 제약으로 보는 경향이 있다. 할 수 있는 것을 제한하는 규칙의 집합으로. OpenClaw의 상황은 반대 관점을 보여준다. **컴플라이언스는 하중을 지탱하는 인프라다**.

도구가 허가되지 않은 통합 경로 위에 구축될 때, 단순히 추상적인 의미에서 규칙을 어기는 것이 아니다. 사용자의 워크플로우를 도구의 통제 밖에 있는 단일 집행 결정으로 언제든 종료될 수 있는 의존성 위에 구축하는 것이다. 그 도구의 모든 사용자가 그 리스크에 노출된다. 그들이 이해하든 못하든.

Tetora의 Anthropic API 약관 준수는 마케팅 포인트가 아니라 설계 요건이다. 자동화는 계속 실행될 때만 유용하다.

## 설정 확인

현재 `claude-code` 모드를 사용 중이라면 다음으로 설정을 확인할 수 있다:

```bash
tetora config show | grep claudeProvider
```

직접 API 모드로 전환하려면 설정을 업데이트하고 키를 등록한다:

```bash
tetora config set claudeProvider anthropic
tetora config set anthropicApiKey sk-ant-...
```

API 키는 `ANTHROPIC_API_KEY` 환경 변수로도 설정할 수 있으며, 설정 필드가 없을 경우 Tetora가 자동으로 읽는다.

두 모드 모두 완전히 지원된다. 선택은 선호도에 달려 있다. 이미 CLI를 사용 중이라면 `claude-code`가 설정이 더 간단하다. `anthropic`은 비용 가시성과 제어가 더 직접적이다.
