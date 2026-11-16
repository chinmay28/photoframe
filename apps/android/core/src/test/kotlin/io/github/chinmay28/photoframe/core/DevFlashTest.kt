package io.github.chinmay28.photoframe.core

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class DevFlashTest {
    private fun near(want: Float, got: Float) = assertEquals(want, got, 0.001f)

    @Test
    fun theVeilFadesInHoldsAndFadesOut() {
        near(0f, DevFlash.veilAlpha(0f))
        near(1f, DevFlash.veilAlpha(0.07f))
        near(1f, DevFlash.veilAlpha(0.5f))
        near(1f, DevFlash.veilAlpha(0.82f))
        near(0f, DevFlash.veilAlpha(1f))
        assertTrue(DevFlash.veilAlpha(0.035f) in 0.01f..0.99f)
        assertTrue(DevFlash.veilAlpha(0.91f) in 0.01f..0.99f)
    }

    @Test
    fun theBadgeOvershootsThenSettles() {
        near(0.82f, DevFlash.badgeScale(0f))
        near(1.02f, DevFlash.badgeScale(0.14f))
        near(1f, DevFlash.badgeScale(0.22f))
        near(1f, DevFlash.badgeScale(0.6f))
        near(1.06f, DevFlash.badgeScale(1f))
        val peak = (0..100).maxOf { DevFlash.badgeScale(it / 100f) }
        assertTrue(peak <= 1.06f + 1e-4f)
    }

    @Test
    fun reducedMotionKeepsTheFadeButNotTheScale() {
        near(1f, DevFlash.badgeScale(0.05f, reducedMotion = true))
        near(1f, DevFlash.veilAlpha(0.5f))
        assertEquals(3_000L, DevFlash.DURATION_MS)
    }
}
