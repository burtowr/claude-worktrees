use anyhow::{Context, Result, bail};
use chrono::{DateTime, Utc};
use rand::Rng;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::process::Command;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum AgentStatus {
    Pending,
    Running,
    Completed,
    Merging,
    Merged,
    Failed,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Agent {
    pub id: String,
    pub branch: String,
    pub worktree: PathBuf,
    pub task: String,
    pub status: AgentStatus,
    pub base_branch: String,
    pub base_commit: String,
    pub created_at: DateTime<Utc>,
    pub merged_at: Option<DateTime<Utc>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct State {
    pub version: String,
    pub repo_root: PathBuf,
    pub worktree_dir: String,
    pub agents: HashMap<String, Agent>,
}

impl State {
    pub fn new(repo_root: PathBuf) -> Self {
        Self {
            version: "1.0".to_string(),
            repo_root,
            worktree_dir: ".worktrees".to_string(),
            agents: HashMap::new(),
        }
    }

    fn state_file(repo_root: &Path) -> PathBuf {
        repo_root.join(".cwt").join("state.json")
    }

    pub fn load(repo_root: &Path) -> Result<Self> {
        let state_file = Self::state_file(repo_root);

        if !state_file.exists() {
            return Ok(Self::new(repo_root.to_path_buf()));
        }

        let content = fs::read_to_string(&state_file)
            .context("Failed to read state file")?;

        serde_json::from_str(&content)
            .context("Failed to parse state file")
    }

    pub fn save(&self) -> Result<()> {
        let state_file = Self::state_file(&self.repo_root);

        // Ensure directory exists
        if let Some(parent) = state_file.parent() {
            fs::create_dir_all(parent)?;
        }

        let content = serde_json::to_string_pretty(&self)?;
        fs::write(&state_file, content)?;
        Ok(())
    }
}

pub struct Manager {
    repo_root: PathBuf,
    state: State,
}

impl Manager {
    pub fn new(repo_root: PathBuf) -> Result<Self> {
        // Verify git repo
        if !repo_root.join(".git").exists() {
            bail!("Not a git repository: {}", repo_root.display());
        }

        let state = State::load(&repo_root)?;

        Ok(Self { repo_root, state })
    }

    /// Run a git command and return output
    fn git(&self, args: &[&str]) -> Result<String> {
        let output = Command::new("git")
            .args(args)
            .current_dir(&self.repo_root)
            .output()
            .context("Failed to run git")?;

        if !output.status.success() {
            let stderr = String::from_utf8_lossy(&output.stderr);
            bail!("Git command failed: {}", stderr);
        }

        Ok(String::from_utf8_lossy(&output.stdout).trim().to_string())
    }

    /// Generate a unique agent ID
    fn generate_id() -> String {
        let timestamp = Utc::now().format("%Y%m%d").to_string();
        let suffix: String = rand::thread_rng()
            .sample_iter(&rand::distributions::Alphanumeric)
            .take(4)
            .map(char::from)
            .collect::<String>()
            .to_lowercase();
        format!("cwt-{}-{}", timestamp, suffix)
    }

    /// Slugify a task description for branch name
    fn slugify(s: &str) -> String {
        let slug: String = s
            .to_lowercase()
            .chars()
            .map(|c| if c.is_alphanumeric() { c } else { '-' })
            .collect();

        // Collapse multiple dashes and trim
        let mut result = String::new();
        let mut last_was_dash = false;
        for c in slug.chars() {
            if c == '-' {
                if !last_was_dash && !result.is_empty() {
                    result.push(c);
                    last_was_dash = true;
                }
            } else {
                result.push(c);
                last_was_dash = false;
            }
        }

        // Trim trailing dash and limit length
        let result = result.trim_end_matches('-').to_string();
        if result.len() > 30 {
            result[..30].to_string()
        } else {
            result
        }
    }

    /// Get current branch name
    pub fn current_branch(&self) -> Result<String> {
        self.git(&["rev-parse", "--abbrev-ref", "HEAD"])
    }

    /// Get current commit SHA
    pub fn current_commit(&self) -> Result<String> {
        self.git(&["rev-parse", "HEAD"])
    }

    /// Create a new worktree for an agent
    pub fn create_worktree(&mut self, task: &str) -> Result<Agent> {
        let id = Self::generate_id();
        let slug = Self::slugify(task);
        let branch = format!("cwt/{}/{}", id, slug);
        let worktree_path = self.repo_root.join(&self.state.worktree_dir).join(&id);

        let base_branch = self.current_branch()?;
        let base_commit = self.current_commit()?;

        // Create worktree with new branch
        self.git(&[
            "worktree",
            "add",
            "-b",
            &branch,
            worktree_path.to_str().unwrap(),
        ])?;

        let agent = Agent {
            id: id.clone(),
            branch,
            worktree: worktree_path,
            task: task.to_string(),
            status: AgentStatus::Running,
            base_branch,
            base_commit,
            created_at: Utc::now(),
            merged_at: None,
        };

        self.state.agents.insert(id, agent.clone());
        self.state.save()?;

        Ok(agent)
    }

    /// Remove a worktree
    pub fn remove_worktree(&mut self, id: &str) -> Result<()> {
        let agent = self.state.agents.get(id)
            .context("Agent not found")?
            .clone();

        // Remove worktree
        let _ = self.git(&["worktree", "remove", agent.worktree.to_str().unwrap()]);
        // Force remove if needed
        let _ = self.git(&["worktree", "remove", "--force", agent.worktree.to_str().unwrap()]);

        // Delete branch
        let _ = self.git(&["branch", "-d", &agent.branch]);
        let _ = self.git(&["branch", "-D", &agent.branch]);

        self.state.agents.remove(id);
        self.state.save()?;
        Ok(())
    }

    /// Get an agent by ID
    pub fn get_agent(&self, id: &str) -> Option<&Agent> {
        self.state.agents.get(id)
    }

    /// List all agents
    pub fn list_agents(&self) -> Vec<&Agent> {
        self.state.agents.values().collect()
    }

    /// Update agent status
    pub fn update_status(&mut self, id: &str, status: AgentStatus) -> Result<()> {
        let agent = self.state.agents.get_mut(id)
            .context("Agent not found")?;
        agent.status = status;
        self.state.save()
    }

    /// Merge an agent's branch
    pub fn merge(&mut self, id: &str) -> Result<()> {
        let agent = self.state.agents.get(id)
            .context("Agent not found")?
            .clone();

        // Update status
        self.update_status(id, AgentStatus::Merging)?;

        // Checkout base branch
        self.git(&["checkout", &agent.base_branch])?;

        // Merge
        let msg = format!("Merge {}: {}", agent.id, agent.task);
        self.git(&["merge", "--no-ff", "-m", &msg, &agent.branch])?;

        // Update status
        if let Some(agent) = self.state.agents.get_mut(id) {
            agent.status = AgentStatus::Merged;
            agent.merged_at = Some(Utc::now());
        }
        self.state.save()
    }

    /// Get repo root
    pub fn repo_root(&self) -> &Path {
        &self.repo_root
    }
}
