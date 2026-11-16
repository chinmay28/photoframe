package io.github.chinmay28.photoframe.core

/**
 * Everything the person chose in the app's settings: which frame.
 *
 * The app is a thin client only. The frame's server drives the panel's
 * hardware (SPI, GPIO, the A–D buttons) on the Pi, so there is nothing for a
 * phone to run itself; the app opens the frame's own web app and adds what
 * a web view lacks.
 */
data class AppConfig(
    /** The frame's address as typed or picked; [Origin.parse] reads it. Blank until set up. */
    val frameAddress: String = "",
) {
    val configured: Boolean get() = frameAddress.isNotBlank()

    /** The origin the web view opens, or a reason it has none. */
    fun origin(): Result<Origin> =
        if (!configured) Result.failure(IllegalStateException("Choose a frame first.")) else Origin.parse(frameAddress)
}

/**
 * What a share from the Gallery or Photos app hands over, filtered to what
 * the app will read: content: addresses only. A file: address would make the
 * app read a path of the sharing app's choosing — its own private files
 * included — and send it to the frame.
 */
object ShareInput {
    /** More than this in one share is refused rather than sent for minutes. */
    const val MAX_FILES = 100

    fun accept(uris: List<String>): List<String> =
        uris.filter { it.startsWith("content://", ignoreCase = true) }.distinct().take(MAX_FILES)
}
