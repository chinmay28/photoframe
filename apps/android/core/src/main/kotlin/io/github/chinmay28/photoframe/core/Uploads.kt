package io.github.chinmay28.photoframe.core

import java.io.InputStream
import java.io.OutputStream
import java.util.Locale

/** What the app does with one shared file before it goes to the frame. */
enum class UploadAs {
    /** A JPEG, PNG, GIF or WebP: the frame reads it as it is. */
    AS_IS,

    /**
     * Another kind of image — above all HEIC, which phone cameras save and
     * the frame can't decode. The phone turns it into a JPEG first.
     */
    JPEG,

    /** Not an image: left out, and said so. */
    REFUSE,
}

/**
 * Decides how each shared file is sent, and under what name.
 *
 * The frame accepts an upload by its file extension (internal/source
 * imageExts) and then checks it decodes, so the name must end in the
 * extension of what is actually sent. A shared file's name is whatever the
 * sharing app chose, often without an extension at all ("IMG_20261116_0912"
 * or "image:1234"), so the type decides and the name follows.
 */
object UploadPlan {
    /** The types the frame decodes, with the extension each is saved under. */
    private val frameTypes = linkedMapOf(
        "image/jpeg" to ".jpg",
        "image/png" to ".png",
        "image/gif" to ".gif",
        "image/webp" to ".webp",
    )
    private val frameExts = mapOf(".jpg" to "image/jpeg", ".jpeg" to "image/jpeg", ".png" to "image/png", ".gif" to "image/gif", ".webp" to "image/webp")
    private val otherImageExts = setOf(".heic", ".heif", ".avif", ".bmp", ".dng", ".tif", ".tiff")

    fun decide(name: String?, mime: String?): UploadAs {
        val m = mime?.lowercase(Locale.ROOT)?.substringBefore(';')?.trim().orEmpty()
        val ext = extOf(name)
        return when {
            m in frameTypes -> UploadAs.AS_IS
            m.startsWith("image/") -> UploadAs.JPEG
            // No useful type (empty, octet-stream): the name is all there is.
            ext in frameExts -> UploadAs.AS_IS
            ext in otherImageExts -> UploadAs.JPEG
            else -> UploadAs.REFUSE
        }
    }

    /** The MIME type the part is sent with. */
    fun mimeFor(name: String?, mime: String?, plan: UploadAs): String {
        val m = mime?.lowercase(Locale.ROOT)?.substringBefore(';')?.trim().orEmpty()
        return when (plan) {
            UploadAs.JPEG -> "image/jpeg"
            else -> if (m in frameTypes) m else frameExts[extOf(name)] ?: "application/octet-stream"
        }
    }

    /**
     * The name the frame saves it under: the shared name without folders or
     * odd characters, with the extension of what is sent. [index] numbers
     * the nameless ones so a batch doesn't arrive as photo, photo-2, ….
     */
    fun nameFor(name: String?, mime: String?, plan: UploadAs, index: Int = 0): String {
        // "image:1234" and other content-provider ids are not names.
        // So is a bare ".jpg", which would otherwise lose its dot as hidden.
        val raw = name?.trim().orEmpty()
        val base = raw.substringAfterLast('/').substringAfterLast('\\')
        val nameless = Regex("^[A-Za-z]+:\\d+$").matches(raw) || base.lastIndexOf('.') == 0
        val clean = if (nameless) "" else FileNames.sanitize(raw, fallback = "")
        val ext = extOf(clean)
        val known = ext.isNotEmpty() && (ext in frameExts || ext in otherImageExts)
        var stem = if (known) clean.dropLast(ext.length) else clean
        if (stem.isBlank()) stem = "shared-photo-${index + 1}"
        val sendMime = mimeFor(name, mime, plan)
        // The name's own spelling of a right extension (".JPEG") is kept.
        val keep = ext in frameExts && frameExts[ext] == sendMime
        return stem + if (keep) clean.takeLast(ext.length) else frameTypes[sendMime] ?: ".jpg"
    }

    /**
     * The longest side a converted photo keeps. The panel is 800×480, and
     * the frame's editor zooms up to 4×, so 3200 pixels across is the most
     * it can ever show; this leaves room over that, and keeps a 48 MP HEIC
     * from being decoded at full size on the phone or sent at full size to a
     * Pi Zero.
     */
    const val MAX_CONVERTED_EDGE = 4096

    /** The size to decode a [width]×[height] image at before converting it. */
    fun convertedSize(width: Int, height: Int): Pair<Int, Int> {
        val long = maxOf(width, height)
        if (long <= MAX_CONVERTED_EDGE || width <= 0 || height <= 0) return width to height
        val f = MAX_CONVERTED_EDGE.toDouble() / long
        return maxOf(1, Math.round(width * f).toInt()) to maxOf(1, Math.round(height * f).toInt())
    }

    private fun extOf(name: String?): String {
        val base = name?.substringAfterLast('/')?.substringAfterLast('\\') ?: return ""
        val dot = base.lastIndexOf('.')
        return if (dot > 0) base.substring(dot).lowercase(Locale.ROOT) else ""
    }
}

/**
 * Writes a multipart/form-data body (RFC 7578), one file part at a time, so
 * a batch of photos streams to the frame without being held in memory.
 *
 * The frame reads the parts named `photos` (internal/api upload), and its
 * test feeds the body this class makes for testdata/android-upload.multipart
 * to the real handler, so the two can't drift apart.
 */
class Multipart(val boundary: String = randomBoundary()) {
    init {
        require(Regex("[0-9A-Za-z'()+_,./:=?-]{1,70}").matches(boundary)) { "not a valid boundary" }
    }

    val contentType: String get() = "multipart/form-data; boundary=$boundary"

    /** One file part: its headers, then [data] copied to [out]. Returns the bytes of data written. */
    fun writeFile(out: OutputStream, field: String, fileName: String, mime: String, data: InputStream): Long {
        val head = "--$boundary\r\n" +
            "Content-Disposition: form-data; name=\"${quote(field)}\"; filename=\"${quote(fileName)}\"\r\n" +
            "Content-Type: $mime\r\n\r\n"
        out.write(head.toByteArray(Charsets.UTF_8))
        val n = data.copyTo(out, 64 * 1024)
        out.write("\r\n".toByteArray(Charsets.US_ASCII))
        return n
    }

    /** The closing delimiter, after the last part. */
    fun finish(out: OutputStream) {
        out.write("--$boundary--\r\n".toByteArray(Charsets.US_ASCII))
    }

    companion object {
        /**
         * A name inside a quoted header value, escaped the way browsers do
         * (the HTML standard's form encoding): `"` and line breaks are
         * percent-encoded, everything else — UTF-8 included — kept as it is.
         */
        fun quote(s: String): String = s.replace("\"", "%22").replace("\r", "%0D").replace("\n", "%0A")

        fun randomBoundary(): String {
            val r = java.security.SecureRandom()
            return "PhotoframeAndroid" + (1..24).map { "0123456789abcdefghijklmnopqrstuvwxyz"[r.nextInt(36)] }.joinToString("")
        }
    }
}
