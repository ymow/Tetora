package main

import (
	"net/http"
	"tetora/internal/pwa"
)

const pwaManifestJSON = pwa.ManifestJSON
const pwaServiceWorkerJS = pwa.ServiceWorkerJS
const pwaIconSVG = pwa.IconSVG

func handlePWAManifest(w http.ResponseWriter, r *http.Request) { pwa.HandleManifest(w, r) }
func handlePWAServiceWorker(w http.ResponseWriter, r *http.Request) {
	pwa.HandleServiceWorker(tetoraVersion)(w, r)
}
func handlePWAIcon(w http.ResponseWriter, r *http.Request) { pwa.HandleIcon(w, r) }
