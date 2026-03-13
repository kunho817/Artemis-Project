"""BigQuery collector for Go source files used in training data generation."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

import yaml
from google.api_core.exceptions import GoogleAPIError
from google.auth.exceptions import DefaultCredentialsError
from google.cloud import bigquery
from tqdm import tqdm


class BigQueryCollector:
    """Collect Go files from public BigQuery datasets and save them as JSONL."""

    def __init__(self, config: dict[str, Any]) -> None:
        """Initialize collector with the ``bigquery`` section from data config.

        Args:
            config: Configuration dictionary for BigQuery collection.
        """
        self.config = config
        self.project_id = config.get("project_id")
        self.dataset = config.get("dataset", "bigquery-public-data.github_repos")
        self.max_file_size = int(config.get("max_file_size_bytes", 102_400))
        self.min_file_size = int(config.get("min_file_size_bytes", 100))
        self.max_files = int(config.get("max_files", 500_000))
        self.languages = list(config.get("languages", ["Go"]))

        quality = config.get("quality", {})
        self.exclude_patterns = [str(p).lower() for p in quality.get("exclude_patterns", [])]
        self.page_size = int(config.get("page_size", 2_000))

        try:
            self.client = bigquery.Client(project=self.project_id)
        except DefaultCredentialsError as exc:
            raise RuntimeError(
                "Google Cloud credentials are not configured. "
                "Set GOOGLE_APPLICATION_CREDENTIALS to a valid service account key file."
            ) from exc

    @staticmethod
    def _to_output_record(path: str, repo: str, content: str, size: int) -> dict[str, Any]:
        """Convert raw row fields to the output JSONL schema."""
        return {
            "path": path,
            "repo": repo,
            "content": content,
            "size": size,
            "language": "Go",
        }

    def _is_excluded(self, path: str) -> bool:
        """Check if a file path should be excluded using configured patterns."""
        lower_path = path.lower()
        return any(pattern and pattern in lower_path for pattern in self.exclude_patterns)

    def collect(self, output_dir: str) -> int:
        """Collect Go files from BigQuery and write them into JSONL.

        Args:
            output_dir: Output directory where the JSONL file will be stored.

        Returns:
            Number of collected files written to disk.

        Raises:
            RuntimeError: If credentials are missing or BigQuery query fails.
        """
        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)
        jsonl_path = output_path / "go_bigquery.jsonl"

        if "Go" not in self.languages:
            return 0

        sql_exclude_clauses: list[str] = []
        query_params: list[bigquery.QueryParameter] = [
            bigquery.ScalarQueryParameter("min_size", "INT64", self.min_file_size),
            bigquery.ScalarQueryParameter("max_size", "INT64", self.max_file_size),
            bigquery.ScalarQueryParameter("max_files", "INT64", self.max_files),
        ]

        for idx, pattern in enumerate(self.exclude_patterns):
            name = f"exclude_{idx}"
            query_params.append(bigquery.ScalarQueryParameter(name, "STRING", f"%{pattern}%"))
            sql_exclude_clauses.append(f"LOWER(f.path) NOT LIKE @{name}")

        excludes_sql = ""
        if sql_exclude_clauses:
            excludes_sql = "\n  AND " + "\n  AND ".join(sql_exclude_clauses)

        query = f"""
SELECT
  f.path AS path,
  f.repo_name AS repo,
  SAFE_CONVERT_BYTES_TO_STRING(c.content) AS content,
  f.size AS size
FROM `{self.dataset}.files` AS f
JOIN `{self.dataset}.contents` AS c
  ON f.id = c.id
WHERE LOWER(f.path) LIKE '%.go'
  AND f.size BETWEEN @min_size AND @max_size
  AND SAFE_CONVERT_BYTES_TO_STRING(c.content) IS NOT NULL
  {excludes_sql}
ORDER BY f.repo_name, f.path
LIMIT @max_files
"""

        job_config = bigquery.QueryJobConfig(query_parameters=query_params)

        try:
            rows = self.client.query(query, job_config=job_config).result(page_size=self.page_size)
        except GoogleAPIError as exc:
            raise RuntimeError(f"BigQuery query failed: {exc}") from exc

        count = 0
        with jsonl_path.open("w", encoding="utf-8") as f_out:
            progress = tqdm(total=self.max_files, desc="Collecting Go files from BigQuery", unit="file")
            for page in rows.pages:
                for row in page:
                    path = str(row.get("path") or "")
                    repo = str(row.get("repo") or "")
                    content = row.get("content")
                    size = int(row.get("size") or 0)

                    if not path or not repo or not isinstance(content, str):
                        continue
                    if not path.lower().endswith(".go"):
                        continue
                    if self._is_excluded(path):
                        continue
                    if size < self.min_file_size or size > self.max_file_size:
                        continue

                    record = self._to_output_record(path=path, repo=repo, content=content, size=size)
                    f_out.write(json.dumps(record, ensure_ascii=False) + "\n")
                    count += 1
                    progress.update(1)

                    if count >= self.max_files:
                        break
                if count >= self.max_files:
                    break
            progress.close()

        return count

    def collect_star_counts(self, output_dir: str) -> dict[str, int]:
        """Collect GitHub repository star counts from GH Archive via BigQuery.

        This method expects optional star-query settings in the BigQuery config:
        - star_start_date: YYYYMMDD
        - star_end_date: YYYYMMDD
        - star_repo_limit: number of Go repos to include

        Args:
            output_dir: Output directory where star count JSON will be saved.

        Returns:
            Mapping of repository name to star count.
        """
        start_date = self.config.get("star_start_date")
        end_date = self.config.get("star_end_date")
        repo_limit = int(self.config.get("star_repo_limit", 100_000))

        if not start_date or not end_date:
            return {}

        query = """
WITH go_repos AS (
  SELECT DISTINCT repo_name
  FROM `bigquery-public-data.github_repos.files`
  WHERE LOWER(path) LIKE '%.go'
  LIMIT @repo_limit
)
SELECT
  repo.name AS repo,
  COUNT(1) AS star_count
FROM `githubarchive.day.*`
WHERE type = 'WatchEvent'
  AND _TABLE_SUFFIX BETWEEN @start_date AND @end_date
  AND repo.name IN (SELECT repo_name FROM go_repos)
GROUP BY repo
"""

        job_config = bigquery.QueryJobConfig(
            query_parameters=[
                bigquery.ScalarQueryParameter("start_date", "STRING", str(start_date)),
                bigquery.ScalarQueryParameter("end_date", "STRING", str(end_date)),
                bigquery.ScalarQueryParameter("repo_limit", "INT64", repo_limit),
            ]
        )

        result: dict[str, int] = {}
        try:
            rows = self.client.query(query, job_config=job_config).result(page_size=self.page_size)
            for row in rows:
                repo = str(row.get("repo") or "")
                stars = int(row.get("star_count") or 0)
                if repo:
                    result[repo] = stars
        except GoogleAPIError as exc:
            raise RuntimeError(f"Failed to collect star counts from GH Archive: {exc}") from exc

        output_path = Path(output_dir)
        output_path.mkdir(parents=True, exist_ok=True)
        stars_file = output_path / "repo_star_counts.json"
        stars_file.write_text(json.dumps(result, ensure_ascii=False, indent=2), encoding="utf-8")
        return result


if __name__ == "__main__":
    config_path = Path("configs/data_config.yaml")
    if not config_path.exists():
        raise FileNotFoundError(f"Config file not found: {config_path}")

    with config_path.open("r", encoding="utf-8") as fp:
        root_config: dict[str, Any] = yaml.safe_load(fp) or {}

    bigquery_config = dict(root_config.get("bigquery", {}))
    quality_config = dict(root_config.get("quality", {}))
    if quality_config:
        bigquery_config["quality"] = quality_config

    collector = BigQueryCollector(bigquery_config)
    collected = collector.collect(output_dir=str(Path("data/raw")))
    print(f"Collected {collected} Go files from BigQuery.")

    if os.getenv("COLLECT_STAR_COUNTS", "0") == "1":
        stars = collector.collect_star_counts(output_dir=str(Path("data/raw")))
        print(f"Collected star counts for {len(stars)} repositories.")
