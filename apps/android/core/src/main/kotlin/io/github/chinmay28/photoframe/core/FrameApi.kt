package io.github.chinmay28.photoframe.core

import org.json.JSONArray
import org.json.JSONObject

/**
 * The few answers of the frame's JSON API that the app reads natively (the
 * page reads the rest). Each reader is forgiving: a frame that answers
 * something unexpected gives null or an empty list, never a crash.
 */
object FrameApi {
    const val STATUS = "/api/status?upNext=0"
    const val SOURCES = "/api/sources"

    fun control(command: String) = "/api/control/$command"
    fun upload(sourceId: String) = "/api/sources/${percentEncode(sourceId)}/photos"

    /** What `GET /api/status` says about the slideshow. */
    data class Status(val name: String, val paused: Boolean, val total: Int)

    fun status(body: String): Status? = runCatching {
        val o = JSONObject(body)
        val p = o.getJSONObject("player")
        Status(o.optString("name"), p.optBoolean("paused", false), p.optInt("total", 0))
    }.getOrNull()

    /** A source a photo can be uploaded into. */
    data class UploadSource(val id: String, val name: String)

    /**
     * Where a shared photo goes: the first enabled upload source, else the
     * first upload source at all — the same choice the page's own Upload
     * button makes (web/app.js), so both land in the same place. Null when
     * the frame has no upload source.
     */
    fun uploadSource(sourcesBody: String): UploadSource? = runCatching {
        val a = JSONArray(sourcesBody)
        val uploads = List(a.length()) { a.getJSONObject(it) }.filter { it.optString("type") == "upload" && it.optString("id").isNotEmpty() }
        val pick = uploads.firstOrNull { it.optBoolean("enabled", false) } ?: uploads.firstOrNull()
        pick?.let { UploadSource(it.getString("id"), it.optString("name").ifBlank { it.getString("id") }) }
    }.getOrNull()

    /** The answer to an upload (internal/api.UploadResult). */
    data class UploadResult(val saved: List<String>, val rejected: List<String>)

    fun uploadResult(body: String): UploadResult? = runCatching {
        val o = JSONObject(body)
        UploadResult(strings(o.optJSONArray("saved")), strings(o.optJSONArray("rejected")))
    }.getOrNull()

    /** The frame's `{"error": "…"}`, or null when the body isn't one. */
    fun error(body: String): String? = runCatching { JSONObject(body).optString("error").ifBlank { null } }.getOrNull()

    private fun strings(a: JSONArray?): List<String> = if (a == null) emptyList() else List(a.length()) { a.getString(it) }

    private fun percentEncode(s: String): String = buildString {
        for (b in s.toByteArray(Charsets.UTF_8)) {
            val c = b.toInt() and 0xff
            if (c.toChar().isLetterOrDigit() && c < 0x80 || c.toChar() in "-._~") append(c.toChar()) else append("%%%02X".format(c))
        }
    }
}

/** The slideshow commands a launcher shortcut sends (`POST /api/control/{cmd}`). */
enum class Control(val action: String) {
    NEXT("io.github.chinmay28.photoframe.action.NEXT"),
    PREVIOUS("io.github.chinmay28.photoframe.action.PREVIOUS"),

    /** Pause when playing, resume when paused: needs the frame's status first. */
    PAUSE_OR_RESUME("io.github.chinmay28.photoframe.action.PAUSE_OR_RESUME");

    /** The command to send, given whether the slideshow is paused now. */
    fun command(paused: Boolean): String = when (this) {
        NEXT -> "next"
        PREVIOUS -> "prev"
        PAUSE_OR_RESUME -> if (paused) "resume" else "pause"
    }

    /** Whether [command] needs the status read first. */
    val needsStatus: Boolean get() = this == PAUSE_OR_RESUME

    companion object {
        fun of(action: String?): Control? = entries.firstOrNull { it.action == action }
    }
}
