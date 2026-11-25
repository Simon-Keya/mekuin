const CACHE = "meku-in-shell-v1";
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", event => event.waitUntil((async()=>{const keys=await caches.keys();await Promise.all(keys.filter(k=>k.startsWith("meku-in-")&&k!==CACHE).map(k=>caches.delete(k)));await self.clients.claim();})()));
self.addEventListener("fetch", event => {
  const request=event.request; const url=new URL(request.url);
  if(request.method!=="GET" || url.origin!==self.location.origin) return;
  if(url.pathname.startsWith("/api/") || url.pathname.startsWith("/auth/") || url.pathname.includes("tenant")) return;
  if(url.pathname.startsWith("/_next/static/")) {
    event.respondWith(caches.open(CACHE).then(async cache=>{const hit=await cache.match(request);if(hit)return hit;const response=await fetch(request);if(response.ok)cache.put(request,response.clone());return response;}));return;
  }
  if(request.mode==="navigate") {
    event.respondWith((async()=>{const cache=await caches.open(CACHE);try{const response=await fetch(request);if(response.ok && (url.pathname==="/pos" || url.pathname==="/app"))await cache.put(url.pathname,response.clone());return response;}catch{const cached=await cache.match(url.pathname);if(cached)return cached;return new Response("Offline page not yet cached. Reconnect and open the POS once.",{status:503,headers:{"Content-Type":"text/plain; charset=utf-8"}});}})());
  }
});
