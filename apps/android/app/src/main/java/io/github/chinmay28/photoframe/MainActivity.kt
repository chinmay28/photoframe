package io.github.chinmay28.photoframe

import android.annotation.SuppressLint
import android.content.ActivityNotFoundException
import android.content.Intent
import android.net.Uri
import android.net.http.SslError
import android.os.Bundle
import android.os.Message
import android.view.View
import android.view.ViewGroup
import android.webkit.MimeTypeMap
import android.webkit.SslErrorHandler
import android.webkit.ValueCallback
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import android.widget.Toast
import androidx.activity.OnBackPressedCallback
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import io.github.chinmay28.photoframe.core.AcceptTypes
import io.github.chinmay28.photoframe.core.AppConfig
import io.github.chinmay28.photoframe.core.Discovery
import io.github.chinmay28.photoframe.core.Navigation
import io.github.chinmay28.photoframe.core.NavigationPolicy
import io.github.chinmay28.photoframe.core.Origin
import kotlin.concurrent.thread

/**
 * The app: the frame's own web app, in a web view.
 *
 * Everything Photoframe does in a browser it does here, because it is the
 * same page: sources, the schedule, quiet hours, cropping, colour tuning,
 * the panel type. The app supplies what a browser would have and a web view
 * does not — a file picker for uploads, a real browser for outside links —
 * and finds a `photoframe.local` the phone's own resolver can't (see
 * [FrameResolver]).
 */
class MainActivity : AppCompatActivity() {
    private lateinit var store: ConfigStore
    private lateinit var config: AppConfig

    /** Where the web view is: the configured frame, as resolved this session. */
    private var origin: Origin? = null

    private lateinit var web: WebView
    private lateinit var statusPanel: View
    private lateinit var statusText: TextView
    private lateinit var statusDetail: TextView
    private lateinit var statusProgress: ProgressBar
    private lateinit var statusButtons: LinearLayout

    private var bridge: PageBridge? = null
    private var loaded = false
    private var pendingFiles: ValueCallback<Array<Uri>>? = null

    private val pickFiles = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) { result ->
        val cb = pendingFiles ?: return@registerForActivityResult
        pendingFiles = null
        val data = result.data
        val uris = if (result.resultCode != RESULT_OK || data == null) {
            null
        } else {
            val clip = data.clipData
            if (clip != null && clip.itemCount > 0) Array(clip.itemCount) { clip.getItemAt(it).uri } else data.data?.let { arrayOf(it) }
        }
        cb.onReceiveValue(uris)
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        store = ConfigStore(this)
        config = store.load()
        if (!config.configured) {
            startActivity(Intent(this, SetupActivity::class.java))
            finish()
            return
        }

        setContentView(R.layout.activity_main)
        web = findViewById(R.id.web)
        statusPanel = findViewById(R.id.status)
        statusText = findViewById(R.id.status_text)
        statusDetail = findViewById(R.id.status_detail)
        statusProgress = findViewById(R.id.status_progress)
        statusButtons = findViewById(R.id.status_buttons)
        findViewById<View>(R.id.retry).setOnClickListener { connect() }
        findViewById<View>(R.id.settings).setOnClickListener { openSettings() }

        // The page lays itself out for the screen it is given; the bars and
        // the keyboard are kept out of it here.
        ViewCompat.setOnApplyWindowInsetsListener(findViewById(R.id.root)) { v, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout() or WindowInsetsCompat.Type.ime())
            v.setPadding(bars.left, bars.top, bars.right, bars.bottom)
            WindowInsetsCompat.CONSUMED
        }

        onBackPressedDispatcher.addCallback(this, object : OnBackPressedCallback(true) {
            override fun handleOnBackPressed() {
                if (loaded && web.canGoBack()) web.goBack() else finish()
            }
        })

        connect()
    }

    override fun onResume() {
        super.onResume()
        if (!::config.isInitialized || !config.configured) return
        // Back from settings with another frame chosen.
        if (store.load() != config) recreate()
    }

    override fun onDestroy() {
        if (::web.isInitialized) {
            (web.parent as? ViewGroup)?.removeView(web)
            web.destroy()
        }
        super.onDestroy()
    }

    // --- finding the frame ---

    /** Resolves the configured frame (a `.local` name may need NSD), then loads it. */
    private fun connect() {
        val configured = config.origin().getOrElse {
            showProblem(it.message ?: "The settings name no frame.", null, retry = false)
            return
        }
        showWaiting(
            if (Discovery.isLocalName(configured.host) && origin == null) getString(R.string.finding, configured.host)
            else getString(R.string.connecting, (origin ?: configured).toString()),
        )
        thread(name = "photoframe-resolve") {
            val resolved = FrameResolver.resolve(this, configured)
            runOnUiThread {
                if (isFinishing || isDestroyed) return@runOnUiThread
                resolved.fold(
                    onSuccess = { o -> open(o) },
                    onFailure = { showProblem(getString(R.string.cannot_find, configured.host), getString(R.string.cannot_find_detail), retry = true) },
                )
            }
        }
    }

    private fun open(o: Origin) {
        val current = origin
        when {
            current == null -> {
                origin = o
                setUpWebView(o)
            }
            // The frame moved (a new DHCP lease): the bridge and the
            // navigation rules are keyed on the old address, so start over.
            current != o -> {
                recreate()
                return
            }
        }
        load()
    }

    // --- the web view ---

    @SuppressLint("SetJavaScriptEnabled")
    private fun setUpWebView(o: Origin) {
        web.settings.apply {
            javaScriptEnabled = true
            domStorageEnabled = true
            setSupportMultipleWindows(true)
            javaScriptCanOpenWindowsAutomatically = true
            allowFileAccess = false
            allowContentAccess = false
            mixedContentMode = WebSettings.MIXED_CONTENT_NEVER_ALLOW
            builtInZoomControls = false
            // The page is laid out for the phone (its viewport tag says
            // device-width); overview mode would zoom out to whatever is
            // widest on it instead.
            loadWithOverviewMode = false
            useWideViewPort = true
        }
        web.settings.userAgentString = web.settings.userAgentString + " PhotoframeApp/" + BuildConfig.VERSION_NAME

        bridge = PageBridge(this, o, ::openSettings).also { it.install(web) }

        web.webViewClient = object : WebViewClient() {
            override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean =
                route(request.url.toString())

            override fun onPageStarted(view: WebView, url: String, favicon: android.graphics.Bitmap?) {
                bridge?.onPageStarted(view, url)
            }

            override fun onPageFinished(view: WebView, url: String) {
                if (o.contains(url) && statusButtons.visibility != View.VISIBLE) {
                    loaded = true
                    hideStatus()
                }
            }

            override fun onReceivedError(view: WebView, request: WebResourceRequest, error: WebResourceError) {
                if (!request.isForMainFrame) return
                loaded = false
                showProblem(getString(R.string.cannot_reach, o.toString()), error.description?.toString(), retry = true)
            }

            @SuppressLint("WebViewClientOnReceivedSslError")
            override fun onReceivedSslError(view: WebView, handler: SslErrorHandler, error: SslError) {
                handler.cancel()
                loaded = false
                showProblem(getString(R.string.bad_certificate, o.toString()), null, retry = true)
            }
        }

        web.webChromeClient = object : WebChromeClient() {
            // The Sources screen's Upload button.
            override fun onShowFileChooser(view: WebView, callback: ValueCallback<Array<Uri>>, params: FileChooserParams): Boolean {
                pendingFiles?.onReceiveValue(null)
                pendingFiles = callback
                val types = AcceptTypes.mimeTypes(params.acceptTypes.orEmpty().toList()) {
                    MimeTypeMap.getSingleton().getMimeTypeFromExtension(it)
                }
                val intent = Intent(Intent.ACTION_GET_CONTENT).apply {
                    addCategory(Intent.CATEGORY_OPENABLE)
                    type = types.singleOrNull() ?: if (types.all { it.startsWith("image/") } && types.isNotEmpty()) "image/*" else "*/*"
                    if (types.size > 1) putExtra(Intent.EXTRA_MIME_TYPES, types.toTypedArray())
                    if (params.mode == FileChooserParams.MODE_OPEN_MULTIPLE) putExtra(Intent.EXTRA_ALLOW_MULTIPLE, true)
                }
                return try {
                    pickFiles.launch(intent)
                    true
                } catch (_: ActivityNotFoundException) {
                    pendingFiles = null
                    false
                }
            }

            // window.open, or a link opened in a new tab: the address goes
            // to the phone, unless it is the frame's own page.
            override fun onCreateWindow(view: WebView, isDialog: Boolean, isUserGesture: Boolean, resultMsg: Message): Boolean {
                val catcher = WebView(this@MainActivity)
                catcher.webViewClient = object : WebViewClient() {
                    private var done = false

                    private fun take(v: WebView, url: String) {
                        if (done || url == "about:blank") return
                        done = true
                        v.stopLoading()
                        if (!route(url)) web.loadUrl(url)
                        v.post { v.destroy() }
                    }

                    override fun shouldOverrideUrlLoading(v: WebView, request: WebResourceRequest): Boolean {
                        take(v, request.url.toString())
                        return true
                    }

                    // Some web views load a new window's first address without asking.
                    override fun onPageStarted(v: WebView, url: String, favicon: android.graphics.Bitmap?) = take(v, url)
                }
                (resultMsg.obj as WebView.WebViewTransport).webView = catcher
                resultMsg.sendToTarget()
                return true
            }
        }
    }

    private fun load() {
        val o = origin ?: return
        showWaiting(getString(R.string.connecting, o.toString()))
        web.visibility = View.VISIBLE
        val current = web.url
        if (current != null && o.contains(current)) web.reload() else web.loadUrl(o.url("/"))
    }

    /** Where a navigation goes; true when the web view must not follow it. */
    private fun route(url: String): Boolean {
        val o = origin ?: return true
        return when (NavigationPolicy.decide(o, url)) {
            Navigation.IN_APP -> false
            Navigation.EXTERNAL -> {
                openExternally(url)
                true
            }
            Navigation.INTENT -> {
                openIntent(url)
                true
            }
            Navigation.BLOCK -> true
        }
    }

    private fun openExternally(url: String) {
        try {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)).addCategory(Intent.CATEGORY_BROWSABLE))
        } catch (_: ActivityNotFoundException) {
            Toast.makeText(this, R.string.no_app_for_link, Toast.LENGTH_SHORT).show()
        }
    }

    /**
     * An intent: link. The page may name an app and a type, never a
     * component: only apps that take links from a browser can be reached
     * this way, exactly as from Chrome.
     */
    private fun openIntent(url: String) {
        val intent = try {
            Intent.parseUri(url, Intent.URI_INTENT_SCHEME)
        } catch (_: Exception) {
            return
        }
        intent.component = null
        intent.selector = null
        intent.addCategory(Intent.CATEGORY_BROWSABLE)
        intent.flags = 0
        try {
            startActivity(intent)
        } catch (_: ActivityNotFoundException) {
            Toast.makeText(this, R.string.no_app_for_link, Toast.LENGTH_SHORT).show()
        }
    }

    private fun openSettings() {
        startActivity(Intent(this, SettingsActivity::class.java))
    }

    // --- the panel in front of the page ---

    private fun showWaiting(text: String) {
        statusPanel.visibility = View.VISIBLE
        statusProgress.visibility = View.VISIBLE
        statusText.text = text
        statusDetail.visibility = View.GONE
        statusButtons.visibility = View.GONE
    }

    private fun showProblem(text: String, detail: String?, retry: Boolean) {
        statusPanel.visibility = View.VISIBLE
        statusProgress.visibility = View.GONE
        statusText.text = text
        statusDetail.text = detail.orEmpty()
        statusDetail.visibility = if (detail.isNullOrBlank()) View.GONE else View.VISIBLE
        statusButtons.visibility = View.VISIBLE
        findViewById<View>(R.id.retry).visibility = if (retry) View.VISIBLE else View.GONE
        if (::web.isInitialized) web.visibility = View.INVISIBLE
    }

    private fun hideStatus() {
        statusPanel.visibility = View.GONE
        web.visibility = View.VISIBLE
    }
}
