package io.github.chinmay28.photoframe.core

import java.util.Locale

/**
 * Finding frames on the Wi-Fi over mDNS / DNS-SD, the client half of the
 * Avahi service file deploy/install.sh writes (`_photoframe._tcp`, named
 * after the Pi's hostname, on the port the service listens on).
 *
 * Android's NsdManager does the asking. This is what the app makes of the
 * answers, and how it reaches a `photoframe.local` name that the phone's
 * own resolver (and so the web view) can't: many Android versions resolve
 * no `.local` names at all, while NSD can always find the service.
 */
object Discovery {
    /** Must match the `<type>` in the service file deploy/install.sh writes. */
    const val SERVICE_TYPE = "_photoframe._tcp."

    /** A frame that answered: what it is called, and where to open it. */
    data class Found(val name: String, val origin: Origin) {
        /**
         * What the address field gets when this frame is picked: its
         * `.local` name when the service is named after a hostname, which
         * keeps working when the Pi's IP address changes, else the address
         * it answered from.
         */
        val address: String
            get() = hostFor(name)?.let { Origin(origin.scheme, it, origin.port).toString() } ?: origin.toString()
    }

    /**
     * An answer NSD resolved, or null when it can't be opened. [host] is the
     * resolved address as Java prints it (maybe "/192.168.1.20", maybe an
     * IPv6 literal with a "%wlan0" scope, which a URL can't carry).
     */
    fun found(serviceName: String, host: String?, port: Int): Found? {
        val h = host?.trim()?.removePrefix("/")?.substringBefore('%')?.lowercase(Locale.ROOT)
        if (h.isNullOrEmpty() || port !in 1..65535) return null
        // A link-local IPv6 address is useless without the scope dropped above.
        if (h.startsWith("fe80:")) return null
        val origin = runCatching { Origin("http", h, port) }.getOrNull() ?: return null
        return Found(serviceName.ifBlank { h }, origin)
    }

    /** Several answers for one frame (IPv4 and IPv6, or two interfaces) as one, IPv4 first. */
    fun merge(found: List<Found>): List<Found> =
        found.groupBy { it.name.lowercase(Locale.ROOT) }
            .map { (_, all) -> all.firstOrNull { !it.origin.host.contains(':') } ?: all.first() }
            .sortedBy { it.name.lowercase(Locale.ROOT) }

    /** Whether [host] is a multicast-DNS name, which the phone may not resolve by itself. */
    fun isLocalName(host: String): Boolean = host.lowercase(Locale.ROOT).trimEnd('.').endsWith(".local")

    /**
     * Whether a service named [serviceName] is the frame at [host]. Avahi
     * names the service after the hostname (`%h`), so "photoframe.local" is
     * the service called "photoframe", in any case.
     */
    fun isServiceFor(serviceName: String, host: String): Boolean {
        if (!isLocalName(host)) return false
        val label = host.lowercase(Locale.ROOT).trimEnd('.').removeSuffix(".local")
        return serviceName.lowercase(Locale.ROOT) == label
    }

    /** "photoframe" → "photoframe.local"; null for a name that isn't a hostname label. */
    fun hostFor(serviceName: String): String? {
        val n = serviceName.lowercase(Locale.ROOT)
        return if (Regex("[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?").matches(n)) "$n.local" else null
    }
}
