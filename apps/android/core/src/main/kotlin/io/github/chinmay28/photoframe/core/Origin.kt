package io.github.chinmay28.photoframe.core

import java.net.URI
import java.util.Locale

/**
 * Where a frame is: a scheme, a host and a port, and nothing else.
 *
 * Everything the app trusts is keyed on this. The web view loads only this
 * origin, the page bridge answers only it, and the app's own requests (a
 * shared photo, a launcher shortcut) go only to it.
 */
data class Origin(val scheme: String, val host: String, val port: Int) {

    init {
        require(scheme == "http" || scheme == "https") { "scheme must be http or https" }
        require(host.isNotEmpty()) { "host must not be empty" }
        require(port in 1..65535) { "port out of range" }
    }

    val secure: Boolean get() = scheme == "https"

    /** The host as it goes into a URL: an IPv6 literal in brackets. */
    private val urlHost: String get() = if (host.contains(':')) "[$host]" else host

    /** "http://photoframe.local" or "http://192.168.1.20:8080", with no trailing slash. */
    override fun toString(): String {
        val defaultPort = if (secure) 443 else 80
        return if (port == defaultPort) "$scheme://$urlHost" else "$scheme://$urlHost:$port"
    }

    /** The address of [path] on this origin. */
    fun url(path: String = "/"): String = toString() + (if (path.startsWith("/")) path else "/$path")

    /** Whether [url] is on this origin — same scheme, host and port. */
    fun contains(url: String): Boolean = of(url) == this

    /** The same frame at another host: what a `.local` name resolved to. */
    fun withHost(other: String): Origin = copy(host = other.removePrefix("[").removeSuffix("]").lowercase(Locale.ROOT))

    companion object {
        /**
         * The port the frame's service listens on (`-listen :80` in
         * deploy/photoframe.service), which is also plain HTTP's own, so
         * "photoframe.local" and "http://photoframe.local" are the same frame.
         */
        const val DEFAULT_PORT = 80

        /** What the setup screen starts with: the name the README installs under. */
        const val DEFAULT_ADDRESS = "photoframe.local"

        /**
         * Reads what somebody typed into "frame address".
         *
         * Forgiving about what people type: no scheme means http, and no port
         * means the scheme's own (80, the frame's). A path, query or fragment
         * is dropped; the app always opens the frame's root. Credentials in
         * the address are refused rather than dropped, since keeping them
         * would put a password in settings and dropping them silently would
         * surprise.
         */
        fun parse(input: String): Result<Origin> {
            val text = input.trim()
            if (text.isEmpty()) return Result.failure(IllegalArgumentException("Enter the frame's address."))
            val withScheme = if (Regex("^[a-zA-Z][a-zA-Z0-9+.-]*://").containsMatchIn(text)) text else "http://$text"
            val uri = try {
                URI(withScheme)
            } catch (e: Exception) {
                return Result.failure(IllegalArgumentException("That is not an address: $text"))
            }
            val scheme = uri.scheme?.lowercase(Locale.ROOT)
            if (scheme != "http" && scheme != "https") {
                return Result.failure(IllegalArgumentException("The address must start with http:// or https://."))
            }
            if (uri.rawUserInfo != null) {
                return Result.failure(IllegalArgumentException("Leave the user name and password out of the address."))
            }
            val host = uri.host?.removePrefix("[")?.removeSuffix("]")?.lowercase(Locale.ROOT)
            if (host.isNullOrEmpty()) {
                return Result.failure(IllegalArgumentException("That address has no host: $text"))
            }
            val port = when {
                uri.port > 0 -> uri.port
                scheme == "https" -> 443
                else -> DEFAULT_PORT
            }
            return runCatching { Origin(scheme, host, port) }
        }

        /** The origin of a full URL, or null when it is not http(s). */
        fun of(url: String): Origin? = try {
            val uri = URI(url)
            val scheme = uri.scheme?.lowercase(Locale.ROOT)
            val host = uri.host?.removePrefix("[")?.removeSuffix("]")?.lowercase(Locale.ROOT)
            if ((scheme != "http" && scheme != "https") || host.isNullOrEmpty()) {
                null
            } else {
                val port = if (uri.port > 0) uri.port else if (scheme == "https") 443 else 80
                Origin(scheme, host, port)
            }
        } catch (_: Exception) {
            null
        }
    }
}
