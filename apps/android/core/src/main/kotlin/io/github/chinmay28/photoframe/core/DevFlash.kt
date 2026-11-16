package io.github.chinmay28.photoframe.core

/**
 * The developer badge's full-screen flash, as a timeline: the same 3 s as
 * the web app's `.dev-veil` and `.dev-lockup` animations (web/app.css), so
 * the native one on the setup and settings screens moves the same way.
 *
 * The veil fades in over the first 7%, holds, and fades out from 82%. The
 * badge lands from 0.82 with a small overshoot (1.02 at 14%, settled at 22%),
 * holds, and drifts out to 1.06 as the veil clears.
 */
object DevFlash {
    const val DURATION_MS = 3_000L

    /** Veil opacity at [t], a fraction 0..1 of the duration. */
    fun veilAlpha(t: Float): Float = when {
        t <= 0f -> 0f
        t < 0.07f -> easeInOut(t / 0.07f)
        t < 0.82f -> 1f
        t < 1f -> 1f - easeInOut((t - 0.82f) / 0.18f)
        else -> 0f
    }

    /** Badge scale at [t]; with [reducedMotion] it stays put, as the CSS's media query has it. */
    fun badgeScale(t: Float, reducedMotion: Boolean = false): Float = when {
        reducedMotion -> 1f
        t <= 0f -> 0.82f
        t < 0.14f -> lerp(0.82f, 1.02f, easeOut(t / 0.14f))
        t < 0.22f -> lerp(1.02f, 1f, easeOut((t - 0.14f) / 0.08f))
        t < 0.82f -> 1f
        t < 1f -> lerp(1f, 1.06f, easeOut((t - 0.82f) / 0.18f))
        else -> 1.06f
    }

    private fun lerp(a: Float, b: Float, f: Float) = a + (b - a) * f

    private fun easeInOut(f: Float) = f * f * (3f - 2f * f)

    // cubic-bezier(.22, 1, .36, 1) is a steep ease-out; a cubic is close enough.
    private fun easeOut(f: Float): Float {
        val r = 1f - f
        return 1f - r * r * r
    }
}
