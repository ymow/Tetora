package main

import "tetora/internal/push"

type PushSubscription = push.Subscription
type PushKeys = push.SubscriptionKeys
type PushNotification = push.Notification
type PushManager = push.Manager

func newPushManager(cfg *Config) *PushManager {
	return push.NewManager(push.Config{
		HistoryDB:       cfg.HistoryDB,
		VAPIDPrivateKey: cfg.Push.VAPIDPrivateKey,
		VAPIDEmail:      cfg.Push.VAPIDEmail,
		TTL:             cfg.Push.TTL,
	})
}
