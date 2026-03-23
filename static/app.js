// ===================================
// Web Crawler & Search Engine - Frontend
// ===================================

(function () {
    "use strict";

    // --- State ---
    let currentPage = 1;
    let currentQuery = "";
    const pageSize = 10;
    let dashboardInterval = null;

    // --- DOM Elements ---
    const navBtns = document.querySelectorAll(".nav-btn");
    const tabContents = document.querySelectorAll(".tab-content");

    // Crawler form
    const crawlForm = document.getElementById("crawl-form");
    const crawlResult = document.getElementById("crawl-result");

    // Dashboard
    const jobsContainer = document.getElementById("jobs-container");
    const jobCount = document.getElementById("job-count");

    // Search
    const searchForm = document.getElementById("search-form");
    const searchInput = document.getElementById("search-input");
    const searchInfo = document.getElementById("search-info");
    const searchResults = document.getElementById("search-results");
    const pagination = document.getElementById("pagination");
    const prevPageBtn = document.getElementById("prev-page");
    const nextPageBtn = document.getElementById("next-page");
    const pageInfo = document.getElementById("page-info");

    // --- Tab Navigation ---
    navBtns.forEach(function (btn) {
        btn.addEventListener("click", function () {
            const tab = btn.dataset.tab;
            switchTab(tab);
        });
    });

    function switchTab(tab) {
        navBtns.forEach(function (b) { b.classList.remove("active"); });
        tabContents.forEach(function (tc) { tc.classList.remove("active"); });

        document.querySelector('[data-tab="' + tab + '"]').classList.add("active");
        document.getElementById("tab-" + tab).classList.add("active");

        // Start/stop dashboard polling.
        if (tab === "dashboard") {
            startDashboardPolling();
        } else {
            stopDashboardPolling();
        }
    }

    // --- Crawl Form ---
    crawlForm.addEventListener("submit", function (e) {
        e.preventDefault();
        startCrawl();
    });

    function startCrawl() {
        var origin = document.getElementById("origin-url").value.trim();
        var depth = parseInt(document.getElementById("max-depth").value, 10);
        var rateLimit = parseInt(document.getElementById("rate-limit").value, 10);
        var queueCap = parseInt(document.getElementById("queue-capacity").value, 10);

        crawlResult.classList.add("hidden");

        fetch("/api/crawl/start", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                origin: origin,
                depth: depth,
                rate_limit: rateLimit,
                queue_capacity: queueCap
            })
        })
        .then(function (resp) { return resp.json(); })
        .then(function (data) {
            if (data.error) {
                showCrawlResult("Error: " + data.error, "error");
            } else {
                showCrawlResult("Job started! ID: " + data.job_id, "success");
                crawlForm.reset();
                document.getElementById("max-depth").value = "2";
                document.getElementById("rate-limit").value = "10";
                document.getElementById("queue-capacity").value = "1000";
            }
        })
        .catch(function (err) {
            showCrawlResult("Network error: " + err.message, "error");
        });
    }

    function showCrawlResult(msg, type) {
        crawlResult.textContent = msg;
        crawlResult.className = "result-msg " + type;
        crawlResult.classList.remove("hidden");
    }

    // --- Dashboard ---
    function startDashboardPolling() {
        fetchJobStatus();
        dashboardInterval = setInterval(fetchJobStatus, 2000);
    }

    function stopDashboardPolling() {
        if (dashboardInterval) {
            clearInterval(dashboardInterval);
            dashboardInterval = null;
        }
    }

    function fetchJobStatus() {
        fetch("/api/crawl/status")
            .then(function (resp) { return resp.json(); })
            .then(function (jobs) {
                renderJobs(jobs || []);
            })
            .catch(function () {
                // Silently ignore polling errors.
            });
    }

    function renderJobs(jobs) {
        jobCount.textContent = jobs.length + " job" + (jobs.length !== 1 ? "s" : "");

        if (jobs.length === 0) {
            jobsContainer.innerHTML = '<p class="empty-state">No active jobs. Start a crawl to see status here.</p>';
            return;
        }

        var html = "";
        jobs.forEach(function (job) {
            html += '<div class="job-card">'
                + '<div class="job-card-header">'
                + '<span class="job-id">' + escapeHtml(job.job_id) + '</span>'
                + '<span class="job-status ' + escapeHtml(job.status) + '">' + escapeHtml(job.status) + '</span>'
                + '</div>'
                + '<div class="job-origin">Origin: ' + escapeHtml(job.origin_url) + '</div>'
                + '<div class="job-metrics">'
                + '<div class="metric"><div class="metric-value">' + job.processed + '</div><div class="metric-label">Processed</div></div>'
                + '<div class="metric"><div class="metric-value">' + job.queue_depth + '</div><div class="metric-label">Queue Depth</div></div>'
                + '<div class="metric"><div class="metric-value">' + (job.throttled ? "⚠️ Yes" : "✅ No") + '</div><div class="metric-label">Throttled</div></div>'
                + '</div>';

            if (job.status === "running") {
                html += '<div class="job-card-footer">'
                    + '<button class="btn-danger" onclick="stopJob(\'' + escapeHtml(job.job_id) + '\')">Stop</button>'
                    + '</div>';
            }

            html += '</div>';
        });

        jobsContainer.innerHTML = html;
    }

    // Expose stopJob globally for onclick handler.
    window.stopJob = function (jobID) {
        fetch("/api/crawl/stop", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ job_id: jobID })
        })
        .then(function (resp) { return resp.json(); })
        .then(function () {
            fetchJobStatus();
        })
        .catch(function (err) {
            alert("Error stopping job: " + err.message);
        });
    };

    // --- Search ---
    searchForm.addEventListener("submit", function (e) {
        e.preventDefault();
        currentQuery = searchInput.value.trim();
        currentPage = 1;
        performSearch();
    });

    prevPageBtn.addEventListener("click", function () {
        if (currentPage > 1) {
            currentPage--;
            performSearch();
        }
    });

    nextPageBtn.addEventListener("click", function () {
        currentPage++;
        performSearch();
    });

    function performSearch() {
        if (!currentQuery) return;

        var url = "/api/search?q=" + encodeURIComponent(currentQuery)
            + "&page=" + currentPage
            + "&size=" + pageSize;

        fetch(url)
            .then(function (resp) { return resp.json(); })
            .then(function (data) {
                if (data.error) {
                    searchResults.innerHTML = '<p class="empty-state">' + escapeHtml(data.error) + '</p>';
                    searchInfo.classList.add("hidden");
                    pagination.classList.add("hidden");
                    return;
                }

                var total = data.total || 0;
                var results = data.results || [];
                var totalPages = Math.ceil(total / pageSize);

                // Search info
                searchInfo.textContent = total + " result" + (total !== 1 ? "s" : "") + " found for \"" + currentQuery + "\"";
                searchInfo.classList.remove("hidden");

                // Results
                if (results.length === 0) {
                    searchResults.innerHTML = '<p class="empty-state">No results found.</p>';
                } else {
                    var html = "";
                    results.forEach(function (r) {
                        html += '<div class="result-item">'
                            + '<div class="result-url"><a href="' + escapeHtml(r.url) + '" target="_blank" rel="noopener">' + escapeHtml(r.url) + '</a></div>'
                            + '<div class="result-meta">'
                            + '<span>Origin: ' + escapeHtml(r.origin_url) + '</span>'
                            + '<span>Depth: ' + r.depth + '</span>'
                            + '<span class="result-score">Score: ' + r.score + '</span>'
                            + '</div>'
                            + '</div>';
                    });
                    searchResults.innerHTML = html;
                }

                // Pagination
                if (totalPages > 1) {
                    pagination.classList.remove("hidden");
                    pageInfo.textContent = "Page " + currentPage + " of " + totalPages;
                    prevPageBtn.disabled = currentPage <= 1;
                    nextPageBtn.disabled = currentPage >= totalPages;
                } else {
                    pagination.classList.add("hidden");
                }
            })
            .catch(function (err) {
                searchResults.innerHTML = '<p class="empty-state">Error: ' + escapeHtml(err.message) + '</p>';
            });
    }

    // --- Utilities ---
    function escapeHtml(str) {
        var div = document.createElement("div");
        div.appendChild(document.createTextNode(str));
        return div.innerHTML;
    }
})();
