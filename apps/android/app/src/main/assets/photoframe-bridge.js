/* Photoframe for Android — what the app adds to the frame's page.

   Runs before the page's own scripts, on the frame's origin only (see
   PageBridge.kt and core/Bridge.kt for the other side). It says the page is
   inside the app, as window.photoframeApp, so the page can offer what only
   the app has, such as its own settings. The page downloads and shares
   nothing, so, unlike SAND Vault's bridge, no files cross here.

   Nothing here runs on any other site, and nothing changes in a browser:
   a page that never looks for window.photoframeApp behaves as before. */
(function () {
  'use strict'
  if (window.photoframeApp) return
  var port = window.PhotoframeAndroid
  if (!port || typeof port.postMessage !== 'function') return

  var nextRe = 1
  var waiting = {}

  port.addEventListener('message', function (event) {
    var reply
    try { reply = JSON.parse(event.data) } catch (e) { return }
    var w = waiting[reply.re]
    if (!w) return
    delete waiting[reply.re]
    if (reply.ok) w.resolve(reply)
    else w.reject(new Error(reply.error || 'The app could not do that.'))
  })

  function call(msg) {
    return new Promise(function (resolve, reject) {
      var re = nextRe++
      waiting[re] = { resolve: resolve, reject: reject }
      msg.re = re
      port.postMessage(JSON.stringify(msg))
    })
  }

  window.photoframeApp = Object.freeze({
    platform: 'android',
    version: "__APP_VERSION__",
    // { platform, version } from the app itself.
    hello: function () { return call({ t: 'hello' }) },
    openSettings: function () { return call({ t: 'settings' }) },
  })
})()
