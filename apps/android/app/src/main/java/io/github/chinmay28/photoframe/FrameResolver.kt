package io.github.chinmay28.photoframe

import android.content.Context
import io.github.chinmay28.photoframe.core.Discovery
import io.github.chinmay28.photoframe.core.Origin
import java.net.InetAddress
import java.util.concurrent.ExecutorCompletionService
import java.util.concurrent.Executors
import java.util.concurrent.TimeUnit

/**
 * Turns the configured frame into an origin the phone can actually reach.
 *
 * An IP address or an ordinary DNS name is used as it is. A `.local` name —
 * "photoframe.local", what the frame is installed as — is asked of the
 * phone's own resolver and of NSD at once, and the first to answer wins:
 * newer Androids resolve `.local` names themselves, many don't, and the web
 * view uses the phone's resolver. When only NSD knows the frame, the app
 * opens it at the address NSD found, for this session.
 */
object FrameResolver {
    class NotFound(val host: String) : Exception("$host is not on this network")

    /** Blocks for up to about [timeoutMs]; call it off the main thread. */
    fun resolve(context: Context, origin: Origin, timeoutMs: Long = 5_000): Result<Origin> {
        if (!Discovery.isLocalName(origin.host)) return Result.success(origin)
        val pool = Executors.newFixedThreadPool(2)
        try {
            val race = ExecutorCompletionService<Origin?>(pool)
            race.submit {
                runCatching { InetAddress.getByName(origin.host) }.getOrNull()?.let { origin }
            }
            race.submit {
                FrameDiscovery.find(context, timeoutMs) { all -> all.any { Discovery.isServiceFor(it.name, origin.host) } }
                    .firstOrNull { Discovery.isServiceFor(it.name, origin.host) }
                    ?.let { Origin(origin.scheme, it.origin.host, it.origin.port) }
            }
            val deadline = System.currentTimeMillis() + timeoutMs + 500
            repeat(2) {
                val left = deadline - System.currentTimeMillis()
                val f = race.poll(left.coerceAtLeast(0), TimeUnit.MILLISECONDS) ?: return Result.failure(NotFound(origin.host))
                runCatching { f.get() }.getOrNull()?.let { return Result.success(it) }
            }
            return Result.failure(NotFound(origin.host))
        } finally {
            pool.shutdownNow()
        }
    }
}
