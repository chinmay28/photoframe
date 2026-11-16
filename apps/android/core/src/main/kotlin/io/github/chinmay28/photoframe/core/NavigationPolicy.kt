package io.github.chinmay28.photoframe.core

import java.util.Locale

/** What the web view does with a page it was asked to open. */
enum class Navigation {
    /** Open it in the app: the frame's own pages. */
    IN_APP,

    /** Hand it to the phone: a link to Immich, the docs, mail. */
    EXTERNAL,

    /**
     * An intent: URL. Launched only after the app has stripped what a page
     * must not choose (an explicit component, a selector) and limited it to
     * apps that accept links from a browser, as Chrome does.
     */
    INTENT,

    /** Refuse it: schemes a page has no business sending the app to. */
    BLOCK,
}

/**
 * Decides where each navigation goes, so that the web view only ever shows
 * the frame's own app.
 */
object NavigationPolicy {

    /** Schemes that reach into the phone rather than out to an app. */
    private val refused = setOf("javascript", "file", "content", "android-app", "chrome", "about", "ws", "wss")

    fun decide(app: Origin, url: String): Navigation {
        val scheme = url.substringBefore(':', "").lowercase(Locale.ROOT)
        return when {
            scheme == "http" || scheme == "https" -> if (app.contains(url)) Navigation.IN_APP else Navigation.EXTERNAL
            // The page's own previews and the blank page a window starts on.
            scheme == "blob" || scheme == "data" || url == "about:blank" -> Navigation.IN_APP
            scheme == "intent" -> Navigation.INTENT
            scheme.isEmpty() || scheme in refused || !Regex("[a-z][a-z0-9+.-]*").matches(scheme) -> Navigation.BLOCK
            // mailto:, tel:, market: and any other app's own scheme; the
            // phone offers it only to apps that take links from a browser.
            else -> Navigation.EXTERNAL
        }
    }
}
