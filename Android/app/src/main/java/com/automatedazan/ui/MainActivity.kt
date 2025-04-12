package com.automatedazan.ui

import android.content.Intent
import android.os.Bundle
import android.view.Menu
import android.view.MenuItem
import android.view.View
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import com.automatedazan.R
import com.automatedazan.data.PreferenceManager
import com.automatedazan.data.model.PrayerTime
import com.automatedazan.data.repository.PrayerTimeRepositoryImpl
import com.automatedazan.databinding.ActivityMainBinding
import com.automatedazan.worker.PrayerTimesSchedulerWorker
import kotlinx.coroutines.flow.catch
import kotlinx.coroutines.launch
import java.time.Duration
import java.time.LocalTime
import java.time.format.DateTimeFormatter

class MainActivity : AppCompatActivity() {
    
    private lateinit var binding: ActivityMainBinding
    private lateinit var repository: PrayerTimeRepositoryImpl
    private var currentPrayerTime: PrayerTime? = null
    private val timeFormatter = DateTimeFormatter.ofPattern("HH:mm")
    
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        binding = ActivityMainBinding.inflate(layoutInflater)
        setContentView(binding.root)
        
        // Set up toolbar
        setSupportActionBar(binding.toolbar)
        
        repository = PrayerTimeRepositoryImpl(applicationContext)
        
        setupUI()
        loadPrayerTimes()
        
        // Check if setup is complete
        if (!PreferenceManager.isSetupComplete()) {
            Toast.makeText(this, R.string.setup_required, Toast.LENGTH_LONG).show()
            // Open settings activity if setup is not complete
            startActivity(Intent(this, SettingsActivity::class.java))
        } else {
            // Schedule daily prayer time updates at 1:00 AM
            PrayerTimesSchedulerWorker.schedule(applicationContext)
        }
    }
    
    private fun setupUI() {
        // Set up refresh button
        binding.refreshFab.setOnClickListener {
            refreshPrayerTimes()
        }
    }
    
    private fun loadPrayerTimes() {
        lifecycleScope.launch {
            try {
                repository.getTodayPrayerTimes()
                    .catch { e -> 
                        Toast.makeText(this@MainActivity, R.string.error_fetching_times, Toast.LENGTH_SHORT).show()
                    }
                    .collect { result ->
                        if (result.isSuccess) {
                            val prayerTime = result.getOrNull()
                            if (prayerTime != null) {
                                updateUIWithPrayerTimes(prayerTime)
                                currentPrayerTime = prayerTime
                            }
                        } else {
                            Toast.makeText(this@MainActivity, R.string.error_fetching_times, Toast.LENGTH_SHORT).show()
                        }
                    }
            } catch (e: Exception) {
                Toast.makeText(this@MainActivity, R.string.error_fetching_times, Toast.LENGTH_SHORT).show()
            }
        }
    }
    
    private fun refreshPrayerTimes() {
        binding.refreshFab.isEnabled = false
        
        lifecycleScope.launch {
            try {
                repository.refreshPrayerTimes()
                    .catch { e -> 
                        binding.refreshFab.isEnabled = true
                        Toast.makeText(this@MainActivity, R.string.error_fetching_times, Toast.LENGTH_SHORT).show()
                    }
                    .collect { result ->
                        binding.refreshFab.isEnabled = true
                        if (result.isSuccess) {
                            val prayerTime = result.getOrNull()
                            if (prayerTime != null) {
                                updateUIWithPrayerTimes(prayerTime)
                                currentPrayerTime = prayerTime
                                Toast.makeText(this@MainActivity, R.string.times_updated, Toast.LENGTH_SHORT).show()
                            }
                        } else {
                            Toast.makeText(this@MainActivity, R.string.error_fetching_times, Toast.LENGTH_SHORT).show()
                        }
                    }
            } catch (e: Exception) {
                binding.refreshFab.isEnabled = true
                Toast.makeText(this@MainActivity, R.string.error_fetching_times, Toast.LENGTH_SHORT).show()
            }
        }
    }
    
    private fun updateUIWithPrayerTimes(prayerTime: PrayerTime) {
        // Update all prayer times in the UI
        binding.fajrTime.text = prayerTime.fajr.format(timeFormatter)
        binding.dhuhrTime.text = prayerTime.dhuhr.format(timeFormatter)
        binding.asrTime.text = prayerTime.asr.format(timeFormatter)
        binding.maghribTime.text = prayerTime.maghrib.format(timeFormatter)
        binding.ishaTime.text = prayerTime.isha.format(timeFormatter)
        
        // Calculate and display next prayer
        val now = LocalTime.now()
        val nextPrayer = prayerTime.getNextPrayerAfter(now)
        
        if (nextPrayer != null) {
            val (prayerName, prayerDateTime) = nextPrayer
            binding.nextPrayerName.text = prayerName
            binding.nextPrayerTime.text = prayerDateTime.format(timeFormatter)
            
            // Calculate time remaining
            val duration = Duration.between(now, prayerDateTime)
            val hours = duration.toHours()
            val minutes = duration.toMinutes() % 60
            
            val timeRemainingText = when {
                hours > 0 -> "$hours hour${if (hours > 1) "s" else ""} $minutes minute${if (minutes > 1) "s" else ""}"
                else -> "$minutes minute${if (minutes > 1) "s" else ""}"
            }
            
            binding.timeRemaining.text = timeRemainingText
        } else {
            // All prayers for today have passed
            binding.nextPrayerName.text = "Fajr (Tomorrow)"
            binding.nextPrayerTime.text = prayerTime.fajr.format(timeFormatter)
            binding.timeRemaining.text = "Tomorrow"
        }
    }
    
    override fun onCreateOptionsMenu(menu: Menu): Boolean {
        menuInflater.inflate(R.menu.main_menu, menu)
        return true
    }
    
    override fun onOptionsItemSelected(item: MenuItem): Boolean {
        return when (item.itemId) {
            R.id.action_settings -> {
                startActivity(Intent(this, SettingsActivity::class.java))
                true
            }
            else -> super.onOptionsItemSelected(item)
        }
    }
    
    override fun onResume() {
        super.onResume()
        // Refresh prayer times when returning to the app
        loadPrayerTimes()
    }
}