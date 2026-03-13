"""Main QLoRA training entrypoint for Qwen2.5-Coder-7B."""

from __future__ import annotations

import argparse
from pathlib import Path
from typing import Any

import torch
import yaml
from datasets import load_dataset
from peft import LoraConfig
from transformers import AutoModelForCausalLM, AutoTokenizer, BitsAndBytesConfig, TrainingArguments
from trl import SFTTrainer

try:
    from .fim import FIMDataCollator
except ImportError:
    from fim import FIMDataCollator


DEFAULT_MODEL_NAME = "Qwen/Qwen2.5-Coder-7B"
FIM_PAD_TOKEN = "<|fim_pad|>"
FIM_PAD_ID = 151662


def _get_required(cfg: dict[str, Any], path: str) -> Any:
    cur: Any = cfg
    for key in path.split("."):
        if not isinstance(cur, dict) or key not in cur:
            raise KeyError(f"Missing required config key: {path}")
        cur = cur[key]
    return cur


def _get_optional(cfg: dict[str, Any], path: str, default: Any) -> Any:
    cur: Any = cfg
    for key in path.split("."):
        if not isinstance(cur, dict) or key not in cur:
            return default
        cur = cur[key]
    return cur


class QLoRATrainer:
    def __init__(self, config: dict):
        self.config = config
        self.model_name = _get_optional(config, "model.base_model", DEFAULT_MODEL_NAME)

    def setup_model(self) -> tuple:
        quant_config = BitsAndBytesConfig(
            load_in_4bit=True,
            bnb_4bit_quant_type="nf4",
            bnb_4bit_compute_dtype=torch.bfloat16,
            bnb_4bit_use_double_quant=True,
        )

        tokenizer = AutoTokenizer.from_pretrained(self.model_name, trust_remote_code=True)
        tokenizer.pad_token = FIM_PAD_TOKEN
        tokenizer.pad_token_id = FIM_PAD_ID

        model = AutoModelForCausalLM.from_pretrained(
            self.model_name,
            quantization_config=quant_config,
            trust_remote_code=True,
            device_map=_get_optional(self.config, "model.device_map", "auto"),
            torch_dtype=torch.bfloat16,
        )
        model.config.use_cache = False
        model.gradient_checkpointing_enable()

        peft_config = LoraConfig(
            r=64,
            lora_alpha=128,
            target_modules="all-linear",
            lora_dropout=0.05,
            bias="none",
            task_type="CAUSAL_LM",
        )

        total_params = sum(p.numel() for p in model.parameters())
        trainable_params = sum(p.numel() for p in model.parameters() if p.requires_grad)
        ratio = (100.0 * trainable_params / total_params) if total_params else 0.0
        print(
            f"Model params: trainable={trainable_params:,} | total={total_params:,} "
            f"| trainable%={ratio:.4f}%"
        )

        return model, tokenizer, peft_config

    def setup_dataset(self, train_path: str, val_path: str) -> tuple:
        dataset_dict = load_dataset(
            "json",
            data_files={"train": train_path, "validation": val_path},
        )
        return dataset_dict["train"], dataset_dict["validation"]

    def _build_training_args(self) -> TrainingArguments:
        output_dir = _get_required(self.config, "training.output_dir")

        return TrainingArguments(
            output_dir=output_dir,
            learning_rate=float(_get_required(self.config, "training.learning_rate")),
            per_device_train_batch_size=int(_get_required(self.config, "training.per_device_train_batch_size")),
            per_device_eval_batch_size=int(_get_required(self.config, "training.per_device_eval_batch_size")),
            gradient_accumulation_steps=int(_get_required(self.config, "training.gradient_accumulation_steps")),
            num_train_epochs=float(_get_required(self.config, "training.num_train_epochs")),
            lr_scheduler_type=str(_get_required(self.config, "training.lr_scheduler_type")),
            warmup_ratio=float(_get_optional(self.config, "training.warmup_ratio", 0.03)),
            weight_decay=float(_get_optional(self.config, "training.weight_decay", 0.0)),
            optim=str(_get_optional(self.config, "training.optim", "paged_adamw_8bit")),
            logging_steps=int(_get_optional(self.config, "training.logging_steps", 10)),
            save_steps=int(_get_optional(self.config, "training.save_steps", 500)),
            eval_steps=int(_get_optional(self.config, "training.eval_steps", 500)),
            evaluation_strategy=str(_get_optional(self.config, "training.evaluation_strategy", "steps")),
            save_strategy=str(_get_optional(self.config, "training.save_strategy", "steps")),
            save_total_limit=int(_get_optional(self.config, "training.save_total_limit", 3)),
            bf16=bool(_get_optional(self.config, "training.bf16", True)),
            fp16=bool(_get_optional(self.config, "training.fp16", False)),
            gradient_checkpointing=True,
            max_grad_norm=float(_get_optional(self.config, "training.max_grad_norm", 1.0)),
            report_to=str(_get_optional(self.config, "training.report_to", "wandb")),
            run_name=_get_optional(self.config, "training.run_name", None),
            dataloader_num_workers=int(_get_optional(self.config, "training.dataloader_num_workers", 4)),
            seed=int(_get_optional(self.config, "training.seed", 42)),
        )

    def _build_trainer(self, train_path: str, val_path: str) -> tuple[SFTTrainer, AutoTokenizer]:
        model, tokenizer, peft_config = self.setup_model()
        train_dataset, val_dataset = self.setup_dataset(train_path, val_path)

        fim_rate = float(_get_optional(self.config, "fim.fim_rate", 0.4))
        fim_spm_rate = float(_get_optional(self.config, "fim.fim_spm_rate", 0.5))
        data_collator = FIMDataCollator(tokenizer=tokenizer, fim_rate=fim_rate, fim_spm_rate=fim_spm_rate)

        trainer = SFTTrainer(
            model=model,
            tokenizer=tokenizer,
            peft_config=peft_config,
            train_dataset=train_dataset,
            eval_dataset=val_dataset,
            dataset_text_field="text",
            max_seq_length=int(_get_optional(self.config, "training.max_seq_length", 2048)),
            packing=bool(_get_optional(self.config, "training.packing", True)),
            data_collator=data_collator,
            args=self._build_training_args(),
        )
        return trainer, tokenizer

    def train(self, train_path: str, val_path: str):
        trainer, tokenizer = self._build_trainer(train_path, val_path)
        trainer.train()

        output_dir = _get_required(self.config, "training.output_dir")
        Path(output_dir).mkdir(parents=True, exist_ok=True)
        trainer.model.save_pretrained(output_dir)
        tokenizer.save_pretrained(output_dir)
        print(f"Saved adapter/tokenizer to: {output_dir}")

    def resume(self, checkpoint_path: str, train_path: str, val_path: str):
        trainer, tokenizer = self._build_trainer(train_path, val_path)
        trainer.train(resume_from_checkpoint=checkpoint_path)

        output_dir = _get_required(self.config, "training.output_dir")
        Path(output_dir).mkdir(parents=True, exist_ok=True)
        trainer.model.save_pretrained(output_dir)
        tokenizer.save_pretrained(output_dir)
        print(f"Resumed from {checkpoint_path} and saved to: {output_dir}")


def _load_config(path: str) -> dict[str, Any]:
    with open(path, "r", encoding="utf-8") as f:
        data = yaml.safe_load(f)
    if not isinstance(data, dict):
        raise ValueError("Config must deserialize into a dictionary.")
    return data


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="QLoRA + FIM training for Qwen2.5-Coder-7B")
    parser.add_argument("--config", required=True, help="Path to parsed training_config.yaml")
    parser.add_argument("--train-path", required=True, help="Path to train JSONL")
    parser.add_argument("--val-path", required=True, help="Path to validation JSONL")
    parser.add_argument("--resume-from", default=None, help="Checkpoint directory to resume from")
    args = parser.parse_args()

    cfg = _load_config(args.config)
    runner = QLoRATrainer(cfg)

    if args.resume_from:
        runner.resume(args.resume_from, args.train_path, args.val_path)
    else:
        runner.train(args.train_path, args.val_path)
