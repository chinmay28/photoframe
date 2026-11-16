package io.github.chinmay28.photoframe

import android.annotation.SuppressLint
import android.net.Uri
import android.os.Handler
import android.os.Looper
import android.webkit.WebView
import androidx.appcompat.app.AppCompatActivity
import androidx.webkit.JavaScriptReplyProxy
import androidx.webkit.WebMessageCompat
import androidx.webkit.WebViewCompat
import androidx.webkit.WebViewFeature
import io.github.chinmay28.photoframe.core.BridgeReply
import io.github.chinmay28.photoframe.core.BridgeRequest
import io.github.chinmay28.photoframe.core.Origin
import org.json.JSONObject

/**
 * What the app adds to the frame's page: assets/photoframe-bridge.js, run
 * before the page's own scripts, and the `PhotoframeAndroid` object it talks
 * through. Together they define `window.photoframeApp`.
 *
 * Both are given to the frame's [origin] alone — the web view itself checks
 * where each page and each message comes from, so a page from anywhere else
 * sees neither. See [BridgeRequest] for what crosses.
 */
class PageBridge(
    private val activity: AppCompatActivity,
    private val origin: Origin,
    private val onOpenSettings: () -> Unit,
) : WebViewCompat.WebMessageListener {

    private val main = Handler(Looper.getMainLooper())

    /** Whether this web view can carry the bridge at all (WebView 82+). */
    val supported: Boolean
        get() = WebViewFeature.isFeatureSupported(WebViewFeature.WEB_MESSAGE_LISTENER)

    private val needsLateInjection: Boolean
        get() = !WebViewFeature.isFeatureSupported(WebViewFeature.DOCUMENT_START_SCRIPT)

    @SuppressLint("RequiresFeature")
    fun install(web: WebView) {
        if (!supported) return
        val rules = setOf(origin.toString())
        WebViewCompat.addWebMessageListener(web, OBJECT_NAME, rules, this)
        if (!needsLateInjection) WebViewCompat.addDocumentStartJavaScript(web, script(), rules)
    }

    /** For web views without document-start scripts: run it as the page starts. */
    fun onPageStarted(web: WebView, url: String) {
        if (supported && needsLateInjection && origin.contains(url)) web.evaluateJavascript(script(), null)
    }

    private fun script(): String {
        val js = activity.assets.open("photoframe-bridge.js").bufferedReader().use { it.readText() }
        return js.replace("\"__APP_VERSION__\"", JSONObject.quote(BuildConfig.VERSION_NAME))
    }

    // Only ever called through the listener install() added, which it adds
    // only where the feature is present.
    @SuppressLint("RequiresFeature")
    override fun onPostMessage(
        view: WebView,
        message: WebMessageCompat,
        sourceOrigin: Uri,
        isMainFrame: Boolean,
        replyProxy: JavaScriptReplyProxy,
    ) {
        if (!isMainFrame) return
        val text = message.data ?: return
        val reply: (String) -> Unit = { r -> main.post { runCatching { replyProxy.postMessage(r) } } }
        val request = BridgeRequest.parse(text).getOrElse {
            reply(BridgeReply.error(BridgeRequest.reOf(text), it.message ?: "bad message"))
            return
        }
        when (request) {
            is BridgeRequest.Hello -> reply(BridgeReply.ok(request.re, "platform" to "android", "version" to BuildConfig.VERSION_NAME))
            is BridgeRequest.OpenSettings -> {
                main.post(onOpenSettings)
                reply(BridgeReply.ok(request.re))
            }
        }
    }

    companion object {
        const val OBJECT_NAME = "PhotoframeAndroid"
    }
}
