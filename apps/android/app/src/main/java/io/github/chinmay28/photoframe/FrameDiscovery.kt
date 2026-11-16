package io.github.chinmay28.photoframe

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import io.github.chinmay28.photoframe.core.Discovery
import java.util.concurrent.CountDownLatch
import java.util.concurrent.LinkedBlockingQueue
import java.util.concurrent.TimeUnit

/**
 * Finds frames on the Wi-Fi with Android's own mDNS (NsdManager): the
 * `_photoframe._tcp` service each frame's Avahi announces. What the answers
 * mean is [Discovery]'s business; this is only the asking.
 */
object FrameDiscovery {
    /**
     * Blocks for up to [timeoutMs], or until [enough] says what was found
     * will do; call it off the main thread. Returns each frame once.
     */
    fun find(context: Context, timeoutMs: Long = 3_000, enough: (List<Discovery.Found>) -> Boolean = { false }): List<Discovery.Found> {
        val nsd = context.applicationContext.getSystemService(NsdManager::class.java) ?: return emptyList()
        val services = LinkedBlockingQueue<NsdServiceInfo>()
        val found = ArrayList<Discovery.Found>()
        val listener = object : NsdManager.DiscoveryListener {
            override fun onServiceFound(info: NsdServiceInfo) {
                services.add(info)
            }
            override fun onServiceLost(info: NsdServiceInfo) = Unit
            override fun onDiscoveryStarted(serviceType: String) = Unit
            override fun onDiscoveryStopped(serviceType: String) = Unit
            override fun onStartDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
            override fun onStopDiscoveryFailed(serviceType: String, errorCode: Int) = Unit
        }
        val started = runCatching {
            nsd.discoverServices(Discovery.SERVICE_TYPE.removeSuffix("."), NsdManager.PROTOCOL_DNS_SD, listener)
        }.isSuccess
        if (!started) return emptyList()
        try {
            val deadline = System.currentTimeMillis() + timeoutMs
            val asked = HashSet<String>()
            while (true) {
                val left = deadline - System.currentTimeMillis()
                if (left <= 0) break
                val info = services.poll(left, TimeUnit.MILLISECONDS) ?: break
                if (!asked.add(info.serviceName)) continue
                // One resolve at a time: before Android 14 a second one while
                // the first runs fails with FAILURE_ALREADY_ACTIVE.
                resolve(nsd, info, left.coerceAtMost(2_000))?.let { found += it }
                if (enough(Discovery.merge(found))) break
            }
        } finally {
            runCatching { nsd.stopServiceDiscovery(listener) }
        }
        return Discovery.merge(found)
    }

    // resolveService and NsdServiceInfo.host are deprecated from Android 14
    // in favour of a callback API that does not exist before it; this app
    // runs from Android 10, and the old calls still work on 14 and 15.
    @Suppress("DEPRECATION")
    private fun resolve(nsd: NsdManager, info: NsdServiceInfo, timeoutMs: Long): Discovery.Found? {
        val done = CountDownLatch(1)
        var result: Discovery.Found? = null
        val ok = runCatching {
            nsd.resolveService(info, object : NsdManager.ResolveListener {
                override fun onResolveFailed(serviceInfo: NsdServiceInfo, errorCode: Int) = done.countDown()
                override fun onServiceResolved(serviceInfo: NsdServiceInfo) {
                    result = Discovery.found(serviceInfo.serviceName, serviceInfo.host?.hostAddress, serviceInfo.port)
                    done.countDown()
                }
            })
        }.isSuccess
        if (ok) done.await(timeoutMs, TimeUnit.MILLISECONDS)
        return result
    }
}
