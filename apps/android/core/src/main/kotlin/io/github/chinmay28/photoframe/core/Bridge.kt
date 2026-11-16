package io.github.chinmay28.photoframe.core

import org.json.JSONObject

/**
 * The messages the frame's page and the app send each other.
 *
 * assets/photoframe-bridge.js, which the app puts into the page at document
 * start, defines `window.photoframeApp` so the page can tell it is inside the
 * app, and turns its calls into these messages. The frame's page downloads
 * and shares nothing, so unlike SAND Vault's bridge there are no file
 * transfers here: only "what are you" and "open the app's settings".
 *
 * Every request carries `re`, a number the page picked; the answer carries
 * the same one. Messages arrive only from the frame's own origin — the web
 * view enforces that, not this code.
 */
sealed class BridgeRequest {
    abstract val re: Int

    /** The page asking what it is running in. */
    data class Hello(override val re: Int) : BridgeRequest()

    /** Open the app's own settings. */
    data class OpenSettings(override val re: Int) : BridgeRequest()

    companion object {
        fun parse(message: String): Result<BridgeRequest> = runCatching {
            val m = JSONObject(message)
            val re = m.optInt("re", 0)
            when (val t = m.optString("t")) {
                "hello" -> Hello(re)
                "settings" -> OpenSettings(re)
                else -> throw IllegalArgumentException("unknown message ${if (t.isEmpty()) "(no type)" else t}")
            }
        }

        /** The `re` of a message that may not parse, so even a refusal can be answered. */
        fun reOf(message: String): Int = runCatching { JSONObject(message).optInt("re", 0) }.getOrDefault(0)
    }
}

/** An answer to a [BridgeRequest]. */
object BridgeReply {
    fun ok(re: Int, vararg extra: Pair<String, Any?>): String {
        val o = JSONObject().put("re", re).put("ok", true)
        for ((k, v) in extra) o.put(k, v)
        return o.toString()
    }

    fun error(re: Int, message: String): String =
        JSONObject().put("re", re).put("ok", false).put("error", message).toString()
}
