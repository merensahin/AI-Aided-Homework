Product Requirements Document (PRD): Real-Time Web Crawler and Search Engine
1. Project Summary
The objective of this project is to build a functional web crawler and a real-time search engine from scratch. The system must run entirely on localhost and be capable of responding to search queries concurrently while the indexing process is active.
+1

2. Architectural Constraints & Rules
Native Library Usage: You MUST NOT use high-level, fully-featured scraping libraries like Scrapy or Beautiful Soup. All core operations (HTTP requests, HTML parsing) must be handled using language-native capabilities (e.g., net/http or urllib).

Concurrency & Data Safety: To allow concurrent search and indexing, thread-safe data structures (such as Mutexes, Channels, or Concurrent Maps) must be utilized to prevent data corruption.

Scalability Assumption: The system should be designed around the assumption that the scale of the crawl is very large, but it does not require multiple machines to run.

3. System Components & Requirements
3.1. Crawler (Indexer)

The Crawler module will traverse and index web pages based on user-defined parameters.

Recursive Crawling: Initiate a web crawl starting from a given origin URL up to a maximum depth of k.

Uniqueness: Implement a "Visited" data structure (e.g., a visited_urls.data file or a Set) to ensure that no page is ever crawled twice.

Back Pressure: The system must manage its own load to avoid overwhelming the local machine. This must be handled via a maximum rate of work or queue depth limits, pausing or throttling operations when limits are reached.

3.2. Search (Search Engine)

Live Results: The search engine must be able to run while the indexer is still active, reflecting newly discovered results immediately.

Query and Output Format: Accept a query string as input and return a list of triples in the format: (relevant_url, origin_url, depth).

Relevancy: Search results must be ranked from highest to lowest using a simple heuristic logic, such as keyword frequency or title matching.

3.3. Data Persistence & Storage (Bonus)

To ensure the system can be resumed after an interruption without starting from scratch, a file-based storage system will be implemented.

Job ID: Generate a unique ID (e.g., [EpochTime_ThreadID]) whenever a new Crawler Job is initiated.

State Management: Store the crawler's state, queued URLs, and logs in a [crawlerId].data file.

Inverted Index: Discovered words must be stored based on their initial letter (e.g., storage/[letter].data) for fast retrieval by the search engine. Each word entry should include its frequency, the relevant URL, origin URL, and depth.

4. User Interface (UI) Requirements
Build a simple Web UI (or CLI) consisting of 3 main views to easily manage and monitor the system:

Crawler Initialization: An interface to input the origin, depth, and back-pressure parameters (rate limits, queue capacity) to start new crawler jobs.

Status Dashboard: A real-time view of active crawler states, displaying metrics such as:

Indexing Progress (processed vs. queued URLs).

Current Queue Depth.

Back-pressure/Throttling status.

Search Page: An interface for users to submit search queries and view paginated, sorted results.

5. Deliverables
Upon completion, the GitHub repository must include the following:

A fully working codebase running locally.

readme.md: Instructions on how to set up and run the project.

product_prd.md: This exact requirements document.

recommendation.md: A 1-2 paragraph architectural recommendation for deploying this crawler into a high-scale production environment.
+1