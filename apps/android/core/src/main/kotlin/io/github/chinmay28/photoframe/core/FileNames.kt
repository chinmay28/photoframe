package io.github.chinmay28.photoframe.core

import java.nio.charset.StandardCharsets

/** Names for files the app hands the frame. */
object FileNames {
    private const val MAX_BYTES = 180

    /**
     * A name safe to hand on: no folders, no control
     * characters, nothing hidden, not too long — the extension kept when the
     * name has to be cut. Never empty.
     */
    fun sanitize(name: String?, fallback: String = "download"): String {
        var s = (name ?: "")
            .substringAfterLast('/')
            .substringAfterLast('\\')
            .filter { !it.isISOControl() && it !in "<>:\"|?*" }
            .trim()
            .trimStart('.')
            .trim()
        if (s.isEmpty()) s = fallback
        if (s.toByteArray(StandardCharsets.UTF_8).size <= MAX_BYTES) return s

        val dot = s.lastIndexOf('.')
        val ext = if (dot > 0 && s.length - dot <= 10) s.substring(dot) else ""
        var stem = s.substring(0, s.length - ext.length)
        while (stem.isNotEmpty() && (stem + ext).toByteArray(StandardCharsets.UTF_8).size > MAX_BYTES) {
            stem = stem.dropLast(1)
        }
        return (stem.trimEnd() + ext).ifEmpty { fallback }
    }
}
