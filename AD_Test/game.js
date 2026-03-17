// AD_Test/game.js

// 게임 상태 관리
const game = {
    antimatter: 10,
    dimensions: {
        1: {
            amount: 0,
            cost: 10,
            multiplier: 1
        }
    },
    lastTick: Date.now()
};

// DOM 요소 캐싱
const ui = {
    antimatter: document.getElementById('antimatter'),
    antimatterPerSec: document.getElementById('antimatter-per-sec'),
    dim1Amount: document.getElementById('dim1-amount'),
    dim1Multiplier: document.getElementById('dim1-multiplier'),
    dim1Cost: document.getElementById('dim1-cost'),
    buyDim1Btn: document.getElementById('buy-dim1')
};

// 화면 업데이트 함수
function updateUI() {
    ui.antimatter.textContent = Math.floor(game.antimatter);
    
    // 제 1 차원 UI 업데이트
    const dim1 = game.dimensions[1];
    ui.dim1Amount.textContent = dim1.amount;
    ui.dim1Multiplier.textContent = dim1.multiplier;
    ui.dim1Cost.textContent = dim1.cost;

    // 버튼 활성화/비활성화 상태 업데이트
    ui.buyDim1Btn.disabled = game.antimatter < dim1.cost;

    // 초당 생산량 계산 및 표시
    const productionPerSec = dim1.amount * dim1.multiplier;
    ui.antimatterPerSec.textContent = productionPerSec;
}

// 차원 구매 로직
function buyDimension1() {
    const dim1 = game.dimensions[1];
    if (game.antimatter >= dim1.cost) {
        game.antimatter -= dim1.cost;
        dim1.amount += 1;
        // 구매 시 비용 증가 (예: 10배씩)
        dim1.cost *= 10;
        updateUI();
    }
}

// 차원 구매 이벤트 리스너 연결
if (ui.buyDim1Btn) {
    ui.buyDim1Btn.addEventListener('click', buyDimension1);
}

// 메인 게임 루프 (requestAnimationFrame 사용)
function gameLoop() {
    const now = Date.now();
    const deltaTime = (now - game.lastTick) / 1000; // 초 단위 시간 변화량
    game.lastTick = now;

    const dim1 = game.dimensions[1];
    const productionPerSec = dim1.amount * dim1.multiplier;
    
    // 시간에 비례해 반물질 생성
    game.antimatter += productionPerSec * deltaTime;
    
    updateUI();
    
    // 다음 프레임 예약
    requestAnimationFrame(gameLoop);
}

// 게임 초기화 및 시작
game.lastTick = Date.now();
requestAnimationFrame(gameLoop);
updateUI();
