package io.github.chinmay28.photoframe

import io.github.chinmay28.photoframe.core.Origin
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL

/**
 * The app's own requests to the frame: a launcher shortcut's command, the
 * share sheet's upload. Blocking; call off the main thread.
 *
 * They carry no Origin header, as curl's don't: the frame's same-origin
 * check (internal/api sameOrigin) refuses only a browser's cross-site
 * request, which these are not.
 */
object FrameHttp {
    class Response(val code: Int, val body: String)

    fun open(origin: Origin, path: String, method: String, readTimeoutMs: Int = 10_000): HttpURLConnection {
        val c = URL(origin.url(path)).openConnection() as HttpURLConnection
        c.requestMethod = method
        c.connectTimeout = 8_000
        c.readTimeout = readTimeoutMs
        c.instanceFollowRedirects = false
        c.setRequestProperty("Accept", "application/json")
        c.setRequestProperty("User-Agent", "PhotoframeApp/" + BuildConfig.VERSION_NAME)
        return c
    }

    fun get(origin: Origin, path: String): Response = finish(open(origin, path, "GET"))

    fun post(origin: Origin, path: String): Response {
        val c = open(origin, path, "POST")
        c.doOutput = true
        c.setFixedLengthStreamingMode(0)
        c.outputStream.close()
        return finish(c)
    }

    /** Reads the answer, error bodies included, and disconnects. */
    fun finish(c: HttpURLConnection): Response = try {
        val code = c.responseCode
        val stream = if (code >= 400) c.errorStream else c.inputStream
        Response(code, stream?.bufferedReader()?.use { it.readText() }.orEmpty())
    } finally {
        c.disconnect()
    }

    /** A one-line reason for a failed request, for a toast or a dialog. */
    fun describe(e: Exception): String = when (e) {
        is java.net.UnknownHostException -> "the frame's name could not be found"
        is java.net.SocketTimeoutException -> "the frame did not answer in time"
        is java.net.ConnectException -> "the frame refused the connection"
        is IOException -> e.message ?: "network error"
        else -> e.message ?: e.javaClass.simpleName
    }
}
